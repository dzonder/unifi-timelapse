# Unifi Timelapse Merger

A Go program that merges and speeds up timelapse videos from Unifi Protect cameras.

## Features

- Finds all video files for a specified camera
- Merges them in chronological order
- Speeds up the timelapse (default: 10x, configurable)
- Uses NVIDIA GPU acceleration (h264_nvenc) by default for faster encoding

## Prerequisites

### Install FFmpeg

**Windows:**
1. Download FFmpeg from https://ffmpeg.org/download.html
   - **Important:** For GPU acceleration, download a build with NVIDIA NVENC support (usually the "gpl-shared" or "gpl" build)
2. Extract to a folder (e.g., `C:\ffmpeg`)
3. Add `C:\ffmpeg\bin` to your system PATH

**Verify installation:**
```powershell
ffmpeg -version
```

## Usage

1. Create a `videos` directory and place your Unifi Protect video files in it
2. Run the program with the camera name using the `-camera` flag:

```powershell
go run main.go -camera "G5 Flex"
```

Or build and run:
```powershell
go build -o unifi-timelapse.exe
.\unifi-timelapse.exe -camera "G5 Flex"
```

**Optional flags:**
- `-ffmpeg <path>`: Specify the full path to ffmpeg.exe if it's not in your PATH:
  ```powershell
  .\unifi-timelapse.exe -camera "G5 Flex" -ffmpeg "C:\ffmpeg-master-latest-win64-gpl-shared\bin\ffmpeg.exe"
  ```
- `-gpu <true|false>`: Enable or disable GPU acceleration (default: `true`). Use `-gpu=false` to use software encoding:
  ```powershell
  .\unifi-timelapse.exe -camera "G5 Flex" -gpu=false
  ```
- `-speed <factor>`: Set the speedup factor for the timelapse (default: `10.0` = 10x speed). For example, use `-speed=5` for 5x speed or `-speed=20` for 20x speed:
  ```powershell
  .\unifi-timelapse.exe -camera "G5 Flex" -speed=5
  ```

**Help:**
```powershell
.\unifi-timelapse.exe -help
```

The program will:
- Find all `.mp4` files starting with the camera name in the `videos` directory
- Create an `inputs.txt` file for ffmpeg
- Merge and speed up the videos (default: 10x speed, configurable with `-speed` flag)
- Output: `{camera-name}_merged_timelapse.mp4` in the current directory

## File Format

The program expects files in the format:
```
{camera-name} {date}, {time} GMT+X - {date}, {time} GMT+X.mp4
```

Examples:
```
G5 Flex 12-30-2025, 21.00.00 GMT+1 - 12-31-2025, 03.00.00 GMT+1.mp4
G5 Flex 1-1-2026, 03.00.00 GMT+1 - 1-1-2026, 09.00.00 GMT+1.mp4
```
