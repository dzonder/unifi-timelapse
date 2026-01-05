// Package main provides a tool to merge and speed up timelapse videos from Unifi Protect cameras.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// videosDir is the directory containing video files to process.
	videosDir = "videos"
	// videoExt is the expected video file extension.
	videoExt = ".mp4"
	// inputsFile is the temporary file used by ffmpeg for concatenation.
	inputsFile = "inputs.txt"
	// datePattern is the regex pattern for extracting dates and times from filenames.
	// Pattern: "M-D-YYYY, HH.MM.SS" or "M-D-YYYY, HH:MM:SS"
	dateTimePattern = `(\d{1,2})-(\d{1,2})-(\d{4}),\s+(\d{2})[.:](\d{2})[.:](\d{2})`
	// dateTimeFormat is the Go time format for parsing dates and times from filenames.
	dateTimeFormat = "1-2-2006, 15:04:05"
	// minSpeedFactor is the minimum allowed speed factor.
	minSpeedFactor = 0.1
	// maxSpeedFactor is the maximum allowed speed factor.
	maxSpeedFactor = 1000.0
)

// exitWithError prints an error message and exits with status code 1.
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	var (
		cameraName = flag.String("camera", "", "Camera name to match video files (required)")
		ffmpegPath = flag.String("ffmpeg", "ffmpeg", "Path to ffmpeg executable (default: \"ffmpeg\" from PATH)")
		useGPU     = flag.Bool("gpu", true, "Use NVIDIA GPU acceleration (h264_nvenc)")
		speed      = flag.Float64("speed", 10.0, "Speedup factor for timelapse (default: 10.0 = 10x speed)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -camera <camera-name> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -camera \"G5 Flex\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -camera \"G5 Flex\" -ffmpeg \"C:\\ffmpeg\\bin\\ffmpeg.exe\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -camera \"G5 Flex\" -gpu=false\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -camera \"G5 Flex\" -speed=5\n", os.Args[0])
	}
	flag.Parse()

	if *cameraName == "" {
		fmt.Fprintf(os.Stderr, "Error: -camera flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if *speed < minSpeedFactor || *speed > maxSpeedFactor {
		exitWithError("speed factor must be between %.1f and %.1f", minSpeedFactor, maxSpeedFactor)
	}

	outputFile := fmt.Sprintf("%s_merged_timelapse.mp4", sanitizeFilename(*cameraName))

	// Find all matching video files
	files, err := findVideoFiles(*cameraName)
	if err != nil {
		exitWithError("finding video files: %v", err)
	}

	if len(files) == 0 {
		exitWithError("no video files found for camera: %s", *cameraName)
	}

	fmt.Printf("Found %d video file(s) for camera: %s\n", len(files), *cameraName)

	// Sort files chronologically by parsing dates from filenames
	sort.Slice(files, func(i, j int) bool {
		dateI := extractDateFromPath(files[i])
		dateJ := extractDateFromPath(files[j])
		return dateI.Before(dateJ)
	})

	// Create inputs.txt file
	if err := createInputsFile(files, inputsFile); err != nil {
		exitWithError("creating inputs file: %v", err)
	}
	defer func() {
		if err := os.Remove(inputsFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temporary file %s: %v\n", inputsFile, err)
		}
	}()

	fmt.Printf("Created %s with %d file(s)\n", inputsFile, len(files))

	// Run ffmpeg
	if err := runFFmpeg(*ffmpegPath, inputsFile, outputFile, *useGPU, *speed); err != nil {
		exitWithError("running ffmpeg: %v", err)
	}

	fmt.Printf("Successfully created: %s\n", outputFile)
}

// findVideoFiles searches the videos directory for all MP4 files that start with the given camera name.
// It returns a slice of absolute file paths, or an error if the directory cannot be walked.
func findVideoFiles(cameraName string) ([]string, error) {
	var files []string

	err := filepath.Walk(videosDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), videoExt) {
			return nil
		}

		filename := filepath.Base(path)
		if strings.HasPrefix(filename, cameraName) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			files = append(files, absPath)
		}

		return nil
	})

	return files, err
}

// createInputsFile creates a temporary file listing all video files for ffmpeg's concat demuxer.
// It normalizes Windows paths and escapes special characters for ffmpeg compatibility.
func createInputsFile(files []string, inputsFile string) error {
	f, err := os.Create(inputsFile)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, file := range files {
		// Convert Windows backslashes to forward slashes for ffmpeg compatibility
		normalized := strings.ReplaceAll(file, "\\", "/")
		// Escape single quotes for ffmpeg
		escaped := strings.ReplaceAll(normalized, "'", "'\\''")
		if _, err := fmt.Fprintf(f, "file '%s'\n", escaped); err != nil {
			return fmt.Errorf("writing to inputs file: %w", err)
		}
	}

	return nil
}

// runFFmpeg executes ffmpeg to concatenate and speed up the video files.
// It uses the concat demuxer for better performance and applies a speed factor to the video.
// If useGPU is true, it uses NVIDIA GPU acceleration (h264_nvenc), otherwise software encoding (libx264).
func runFFmpeg(ffmpegPath, inputsFile, outputFile string, useGPU bool, speed float64) error {
	// Use concat demuxer for better performance
	// Speed up by specified factor (setpts=1/speed*PTS)
	speedFactor := 1.0 / speed
	args := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", inputsFile,
		"-filter_complex", fmt.Sprintf("[0:v]setpts=%.6f*PTS[v]", speedFactor),
		"-map", "[v]",
	}

	if useGPU {
		// NVIDIA GPU acceleration
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p4", "-cq", "23")
		fmt.Printf("Running ffmpeg with GPU acceleration from: %s\n", ffmpegPath)
	} else {
		// Software encoding
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "23")
		fmt.Printf("Running ffmpeg with software encoding from: %s\n", ffmpegPath)
	}

	args = append(args, "-pix_fmt", "yuv420p", "-y", outputFile)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// extractDateFromPath extracts the date and time from a video filename for chronological sorting.
// It looks for a date-time pattern (M-D-YYYY, HH.MM.SS or M-D-YYYY, HH:MM:SS) in the filename.
// If parsing fails, it falls back to the file's modification time. Returns the zero time if all methods fail.
// Pattern: "Camera Name M-D-YYYY, HH.MM.SS GMT+X - M-D-YYYY, HH.MM.SS GMT+X"
func extractDateFromPath(filePath string) time.Time {
	filename := filepath.Base(filePath)
	// Extract the first date and time in format M-D-YYYY, HH.MM.SS or M-D-YYYY, HH:MM:SS
	re := regexp.MustCompile(dateTimePattern)
	matches := re.FindStringSubmatch(filename)
	if len(matches) == 7 {
		// Reconstruct the date-time string, normalizing time separators to colons
		dateTimeStr := fmt.Sprintf("%s-%s-%s, %s:%s:%s", matches[1], matches[2], matches[3], matches[4], matches[5], matches[6])
		t, err := time.Parse(dateTimeFormat, dateTimeStr)
		if err == nil {
			return t
		}
	}
	// Fallback to file modification time if parsing fails
	if info, err := os.Stat(filePath); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// sanitizeFilename removes or replaces invalid filename characters with underscores.
// It handles Windows-invalid characters: / \ : * ? " < > |
func sanitizeFilename(name string) string {
	// Replace invalid filename characters
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	return strings.TrimSpace(result)
}
