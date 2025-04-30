# FFmpeg MCP Service

An MCP server providing FFmpeg tools (frame extraction, info, transcode) over stdio using JSON-RPC.

一个通过 stdio 使用 JSON-RPC 提供 FFmpeg 工具（帧提取、信息获取、转码）的 MCP 服务器。

## Installation / 安装

Ensure you have Go installed (version 1.24+ recommended).
确保您已安装 Go（推荐 1.24+ 版本）。

1.  **Clone the repository (if applicable):**
    **克隆仓库（如果适用）：**
    ```bash
    # git clone <your-repo-url>
    # cd ffmpeg-mcp
    ```

2.  **Install dependencies:**
    **安装依赖：**
    ```bash
    go mod tidy
    ```

3.  **Build the executable:**
    **构建可执行文件：**
    ```bash
    go build -o ffmpeg-mcp main.go
    ```
    This will create an executable file named `ffmpeg-mcp` in the current directory.
    这将在当前目录中创建一个名为 `ffmpeg-mcp` 的可执行文件。

## Usage / 使用方法

The server runs over standard input/output (stdio) and expects JSON-RPC 2.0 messages.
该服务器通过标准输入/输出（stdio）运行，并期望接收 JSON-RPC 2.0 消息。

```bash
./ffmpeg-mcp [flags]
```

**Flags / 标志:**

*   `--bin-path PATH`: Path to the directory containing the `ffmpeg` and `ffprobe` executables. If not provided, the server will rely on finding `ffmpeg` and `ffprobe` in the system's PATH environment variable.
    `--bin-path PATH`：包含 `ffmpeg` 和 `ffprobe` 可执行文件的目录路径。如果未提供，服务器将依赖于在系统的 PATH 环境变量中查找 `ffmpeg` 和 `ffprobe`。
*   `--help`: Show help information.
    `--help`：显示帮助信息。

**Example / 示例:**

```bash
# Rely on system PATH for ffmpeg/ffprobe
./ffmpeg-mcp

# Specify the path to the directory containing ffmpeg/ffprobe
./ffmpeg-mcp --bin-path /opt/homebrew/bin/
```

Once running, the server will print `Starting FFmpeg MCP server via stdio...` and wait for JSON-RPC requests on its standard input. Responses will be printed to standard output.
运行后，服务器将打印 `Starting FFmpeg MCP server via stdio...` 并等待其标准输入上的 JSON-RPC 请求。响应将打印到标准输出。

## JSON-RPC Interaction / JSON-RPC 交互

Send JSON-RPC 2.0 requests to the server's stdin. All tool calls use the fixed method name `"tools/call"`.
将 JSON-RPC 2.0 请求发送到服务器的 stdin。所有工具调用都使用固定的方法名 `"tools/call"`。

**General Request Format / 通用请求格式:**

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "<tool_name>",
    "arguments": {
      "<arg1_name>": "<arg1_value>",
      "<arg2_name>": "<arg2_value>",
      ...
    }
  },
  "id": "<your_request_id>"
}
```

**General Success Response Format / 通用成功响应格式:**

```json
{
  "jsonrpc": "2.0",
  "result": {
    "content": [
      {
        "type": "text", // Or other types like "json" if implemented
        "text": "<tool_output_string>" // The result from the tool
      }
      // Multiple content parts possible in MCP, but current tools return one
    ],
    "isError": false
  },
  "id": "<your_request_id>"
}
```

**General Error Response Format / 通用错误响应格式:**

```json
{
  "jsonrpc": "2.0",
  "result": {
    "content": [
      {
        "type": "text", // Error message is usually text
        "text": "<detailed_error_message_including_stderr>"
      }
    ],
    "isError": true
  },
  "id": "<your_request_id>"
}
// Note: JSON-RPC level errors (parsing, method not found) use a different 'error' field at the top level.
// 注意：JSON-RPC 级别的错误（解析、方法未找到）在顶层使用不同的 'error' 字段。
```

## Available Tools / 可用工具

### 1. `extract_frames`

*   **Description:** Extracts frames from a video file using 'ffmpeg -i input -vsync 0 output_pattern'. Creates a directory based on the output pattern.
    从视频文件中提取帧，使用 'ffmpeg -i input -vsync 0 output_pattern'。根据输出模式创建目录。
*   **Arguments:**
    *   `input_path` (string, **required**): Path to the input video file. / 输入视频文件的路径。
    *   `output_pattern` (string, **required**): Output pattern for frames (e.g., 'output/frame_%d.png'). The directory will be created. / 帧的输出模式（例如 'output/frame_%d.png'）。目录将被创建。
*   **Example Request / 请求示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {
        "name": "extract_frames",
        "arguments": {
          "input_path": "/path/to/your/video.mp4",
          "output_pattern": "/path/to/output_dir/frame_%04d.jpg"
        }
      },
      "id": "extract-req-1"
    }
    ```
*   **Example Success Response / 成功响应示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "Successfully extracted frames from '/path/to/your/video.mp4' to '/path/to/output_dir/frame_%04d.jpg'"
          }
        ],
        "isError": false
      },
      "id": "extract-req-1"
    }
    ```
*   **Example Error Response / 错误响应示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "ffmpeg execution failed for frame extraction: exit status 1\nFFmpeg Stderr:\n/path/to/your/video.mp4: No such file or directory\n"
          }
        ],
        "isError": true
      },
      "id": "extract-req-1"
    }
    ```

### 2. `get_frame_info`

*   **Description:** Gets PTS (Presentation Timestamp) and Frame Type (pict_type) for each frame in a video using ffprobe.
    使用 ffprobe 获取视频中每个帧的 PTS（显示时间戳）和帧类型（pict_type）。
*   **Arguments:**
    *   `input_path` (string, **required**): Path to the input video file. / 输入视频文件的路径。
*   **Example Request / 请求示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {
        "name": "get_media_info",
        "arguments": {
          "input_path": "/path/to/your/video.mp4"
        }
      },
      "id": "getinfo-req-1"
    }
    ```
*   **Example Success Response / 成功响应示例:** (Output is a JSON string) / （输出是 JSON 字符串）
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "[{\"pts\":0,\"pts_time\":\"0.000000\",\"pict_type\":\"I\"},{\"pts\":0,\"pts_time\":\"0.040000\",\"pict_type\":\"P\"},...]"
          }
        ],
        "isError": false
      },
      "id": "getinfo-req-1"
    }
    ```
*   **Example Error Response / 错误响应示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "ffprobe execution failed: exit status 1\nFFprobe Stderr:\n/path/to/your/video.mp4: Invalid data found when processing input\n"
          }
        ],
        "isError": true
      },
      "id": "getinfo-req-1"
    }
    ```

### 3. `transcode_push`

*   **Description:** Transcodes a video/stream and pushes it to an output URL. Includes basic streaming options like `-re` and reconnection flags.
    转码视频/流并将其推送到输出 URL。包括基本的流式传输选项，如 `-re` 和重连标志。
*   **Arguments:**
    *   `input_path` (string, **required**): Path/URL to the input video or stream. / 输入视频或流的路径/URL。
    *   `output_url` (string, **required**): Output URL (e.g., 'rtmp://server/live/stream', 'output.mp4'). / 输出 URL（例如 'rtmp://server/live/stream', 'output.mp4'）。
    *   `codec_video` (string, *optional*): Video codec (e.g., 'libx264', 'copy'). Defaults to 'copy'. / 视频编解码器（例如 'libx264', 'copy'）。默认为 'copy'。
    *   `codec_audio` (string, *optional*): Audio codec (e.g., 'aac', 'copy'). Defaults to 'copy'. / 音频编解码器（例如 'aac', 'copy'）。默认为 'copy'。
    *   `format` (string, *optional*): Output container format (e.g., 'flv', 'mp4'). Defaults to 'flv' if output_url starts with 'rtmp://', otherwise FFmpeg often guesses from output_url. / 输出容器格式（例如 'flv', 'mp4'）。如果 output_url 以 'rtmp://' 开头，则默认为 'flv'，否则 FFmpeg 通常会从 output_url 推断。
    *   `bitrate_video` (string, *optional*): Video bitrate (e.g., '2000k'). / 视频比特率（例如 '2000k'）。
*   **Example Request (Transcode to file) / 请求示例 (转码到文件):**
    ```json
    {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {
        "name": "transcode_push",
        "arguments": {
          "input_path": "/path/to/your/video.mp4",
          "output_url": "/path/to/output/video.flv",
          "codec_video": "libx264",
          "codec_audio": "aac",
          "bitrate_video": "1500k",
          "format": "flv"
        }
      },
      "id": "transcode-file-1"
    }
    ```
*   **Example Request (Push to RTMP) / 请求示例 (推送到 RTMP):**
    ```json
    {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {
        "name": "transcode_push",
        "arguments": {
          "input_path": "/path/to/your/video.mp4",
          "output_url": "rtmp://your-server.com/live/streamkey"
        }
      },
      "id": "transcode-rtmp-1"
    }
    ```
*   **Example Success Response / 成功响应示例:** (Note: For streaming, this indicates the process started, it might run for a long time.) / （注意：对于流式传输，这表示进程已启动，它可能会运行很长时间。）
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "Successfully started transcode/push from '/path/to/your/video.mp4' to 'rtmp://your-server.com/live/streamkey'"
          }
        ],
        "isError": false
      },
      "id": "transcode-rtmp-1"
    }
    ```
*   **Example Error Response / 错误响应示例:**
    ```json
    {
      "jsonrpc": "2.0",
      "result": {
        "content": [
          {
            "type": "text",
            "text": "ffmpeg execution failed for transcode/push: exit status 1\nFFmpeg Stderr:\nrtmp://your-server.com/live/streamkey: Connection refused\n"
          }
        ],
        "isError": true
      },
      "id": "transcode-rtmp-1"
    }
    ```
