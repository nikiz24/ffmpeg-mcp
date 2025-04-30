package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// --- Cobra Setup Start ---
var rootCmd = &cobra.Command{
	Use:   "ffmpeg-mcp",
	Short: "An MCP server providing FFmpeg tools over stdio.",
	Long:  `An MCP server providing FFmpeg tools (frame extraction, info, transcode) over stdio using JSON-RPC.`,
	Run:   runServer,
}

var binPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&binPath, "bin-path", "", "Path to the directory containing ffmpeg and ffprobe executables (default: rely on system PATH)")
}

// --- Cobra Setup End ---

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	if binPath == "" {
		log.Printf("Using system PATH for ffmpeg and ffprobe")
	} else {
		log.Printf("Using binary path: %s for ffmpeg and ffprobe", binPath)
	}

	s := server.NewMCPServer(
		"FFmpeg MCP Service 🎬",
		"0.1.0",
		server.WithRecovery(),
	)

	extractFramesTool := mcp.NewTool("extract_frames",
		mcp.WithDescription("Extracts frames from a video file using 'ffmpeg -i input -vsync 0 output_pattern'. Creates a directory based on the output pattern."),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path to the input video file."),
		),
		mcp.WithString("output_pattern",
			mcp.Required(),
			mcp.Description("Output pattern for frames (e.g., 'output/frame_%d.png'). The directory will be created."),
		),
	)
	s.AddTool(extractFramesTool, handleExtractFrames)

	getMediaInfoTool := mcp.NewTool("get_media_info",
		mcp.WithDescription("Gets comprehensive media information including format, streams (codec, profile, resolution, etc.), and frame details (PTS, type, resolution, side_data) using ffprobe."),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path to the input media file."),
		),
	)
	s.AddTool(getMediaInfoTool, handleGetMediaInfo)

	transcodePushTool := mcp.NewTool("transcode_push",
		mcp.WithDescription("Transcodes a video/stream and pushes it to an output URL."),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path/URL to the input video or stream."),
		),
		mcp.WithString("output_url",
			mcp.Required(),
			mcp.Description("Output URL (e.g., 'rtmp://server/live/stream', 'output.mp4')."),
		),
		mcp.WithString("codec_video",
			mcp.Description("Video codec (e.g., 'libx264', 'copy'). Defaults to 'copy'."),
		),
		mcp.WithString("codec_audio",
			mcp.Description("Audio codec (e.g., 'aac', 'copy'). Defaults to 'copy'."),
		),
		mcp.WithString("format",
			mcp.Description("Output container format (e.g., 'flv', 'mp4'). FFmpeg often guesses from output_url."),
		),
		mcp.WithString("bitrate_video",
			mcp.Description("Video bitrate (e.g., '2000k')."),
		),
	)
	s.AddTool(transcodePushTool, handleTranscodePush)

	fmt.Println("Starting FFmpeg MCP server via stdio...")
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v\n", err)
	}
}

func getExecutablePath(baseName string) string {
	if binPath == "" {
		return baseName
	}
	return filepath.Join(binPath, baseName)
}

func handleExtractFrames(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	inputPath, ok := request.Params.Arguments["input_path"].(string)
	if !ok || inputPath == "" {
		return mcp.NewToolResultError("Missing or invalid 'input_path' parameter"), nil
	}
	outputPattern, ok := request.Params.Arguments["output_pattern"].(string)
	if !ok || outputPattern == "" {
		return mcp.NewToolResultError("Missing or invalid 'output_pattern' parameter"), nil
	}

	outputDir := filepath.Dir(outputPattern)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		errMsg := fmt.Sprintf("Failed to create output directory '%s': %v", outputDir, err)
		return mcp.NewToolResultError(errMsg), nil
	}

	log.Printf("Attempting to extract frames: input='%s', output='%s'", inputPath, outputPattern)

	var stderr bytes.Buffer
	ffStream := ffmpeg.Input(inputPath).
		Output(outputPattern, ffmpeg.KwArgs{"vsync": "0"}).
		OverWriteOutput()

	cmd := ffStream.Compile()

	cmd.Path = getExecutablePath("ffmpeg")
	cmd.Stderr = &stderr

	log.Printf("Running command: %v", cmd.Args)

	err := cmd.Run()

	if err != nil {
		errMsg := fmt.Sprintf("ffmpeg execution failed for frame extraction: %v", err)

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			errMsg = fmt.Sprintf("%s\nFFmpeg Stderr:\n%s", errMsg, stderr.String())
		}

		log.Printf("Error: %s", errMsg)
		return mcp.NewToolResultError(errMsg), nil
	}

	log.Printf("Frame extraction successful: input='%s', output='%s'", inputPath, outputPattern)
	return mcp.NewToolResultText(fmt.Sprintf("Successfully extracted frames from '%s' to '%s'", inputPath, outputPattern)), nil
}

// --- Structures for Media Info ---

// StreamInfo holds details about a single media stream.
type StreamInfo struct {
	Index      int               `json:"index"`
	CodecType  string            `json:"codec_type"`             // e.g., "video", "audio"
	CodecName  string            `json:"codec_name"`             // e.g., "h264", "aac"
	Profile    string            `json:"profile,omitempty"`      // e.g., "High", "LC"
	Width      int               `json:"width,omitempty"`        // Video only
	Height     int               `json:"height,omitempty"`       // Video only
	HasBFrames int               `json:"has_b_frames,omitempty"` // Video only (indicates potential frame reordering)
	SampleRate string            `json:"sample_rate,omitempty"`  // Audio only
	Channels   int               `json:"channels,omitempty"`     // Audio only
	Tags       map[string]string `json:"tags,omitempty"`         // Stream tags
}

// FrameInfo holds details about a single decoded frame.
type FrameInfo struct {
	StreamIndex  int                      `json:"stream_index"`
	MediaType    string                   `json:"media_type"`               // "audio" or "video"
	Pts          float64                  `json:"pts"`                      // Presentation Time Stamp (seconds)
	PtsTime      string                   `json:"pts_time"`                 // Presentation Time Stamp (formatted)
	PictType     string                   `json:"pict_type,omitempty"`      // Video only (I, P, B, etc.)
	Width        int                      `json:"width,omitempty"`          // Video only (can change mid-stream)
	Height       int                      `json:"height,omitempty"`         // Video only (can change mid-stream)
	PktPts       float64                  `json:"pkt_pts"`                  // Packet PTS (seconds)
	PktDts       float64                  `json:"pkt_dts"`                  // Packet DTS (seconds)
	SideDataList []map[string]interface{} `json:"side_data_list,omitempty"` // Potential SEI or other side data
}

// MediaInfoResult is the top-level structure returned by the tool.
type MediaInfoResult struct {
	Filename string       `json:"filename"`
	Duration string       `json:"duration"` // Format duration string
	Streams  []StreamInfo `json:"streams"`
	Frames   []FrameInfo  `json:"frames"`
}

// FFprobeOutput maps directly to the JSON output structure from ffprobe.
type FFprobeOutput struct {
	Format *struct {
		Filename string            `json:"filename"`
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []*struct {
		Index      int               `json:"index"`
		CodecType  string            `json:"codec_type"`
		CodecName  string            `json:"codec_name"`
		Profile    string            `json:"profile"`
		Width      int               `json:"width"`
		Height     int               `json:"height"`
		HasBFrames int               `json:"has_b_frames"`
		SampleRate string            `json:"sample_rate"`
		Channels   int               `json:"channels"`
		Tags       map[string]string `json:"tags"`
	} `json:"streams"`
	Frames []*struct {
		StreamIndex  int                      `json:"stream_index"`
		MediaType    string                   `json:"media_type"`
		Pts          float64                  `json:"pts"`
		PtsTime      string                   `json:"pts_time"`
		PictType     string                   `json:"pict_type"`
		Width        int                      `json:"width"`
		Height       int                      `json:"height"`
		PktPts       float64                  `json:"pkt_pts"`
		PktDts       float64                  `json:"pkt_dts"`
		SideDataList []map[string]interface{} `json:"side_data_list"`
	} `json:"frames"`
}

// handleGetMediaInfo replaces handleGetFrameInfo. It fetches detailed media info.
func handleGetMediaInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	inputPath, ok := request.Params.Arguments["input_path"].(string)
	if !ok || inputPath == "" {
		return mcp.NewToolResultError("Missing or invalid 'input_path' parameter"), nil
	}

	ffprobeCmdPath := getExecutablePath("ffprobe")

	log.Printf("Attempting to get media info for: '%s' using ffprobe at %s", inputPath, ffprobeCmdPath)

	var outb, errb bytes.Buffer

	// Comprehensive ffprobe command
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",  // Get format info (filename, duration)
		"-show_streams", // Get stream info (codecs, resolution, etc.)
		"-show_frames",  // Get frame info (pts, type, side_data)
		// "-show_entries",
		// "stream=index,codec_name,codec_type,profile,width,height,sample_rate,channels,has_b_frames,tags" + // Stream fields
		// 	":format=filename,duration,tags" + // Format fields
		// 	":frame=stream_index,media_type,pts,pts_time,pict_type,width,height,pkt_pts,pkt_dts,side_data_list", // Frame fields
		inputPath,
	}
	cmd := exec.CommandContext(ctx, ffprobeCmdPath, args...)
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	log.Printf("Running command: %s %v", cmd.Path, cmd.Args)

	err := cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("ffprobe execution failed: %v\nFFprobe Stderr:\n%s", err, errb.String())
		log.Printf("Error: %s", errMsg)
		return mcp.NewToolResultError(errMsg), nil
	}

	data := outb.Bytes()
	log.Printf("ffprobe raw output length: %d bytes", len(data))
	// For debugging, uncomment the next line:
	// log.Printf("ffprobe raw output: %s", string(data))

	var probeData FFprobeOutput
	if err := json.Unmarshal(data, &probeData); err != nil {
		errMsg := fmt.Sprintf("Failed to parse ffprobe JSON output: %v\nFFprobe JSON Output:\n%s\nFFprobe Stderr:\n%s", err, string(data), errb.String())
		log.Printf("Error: %s", errMsg)
		return mcp.NewToolResultError(errMsg), nil
	}

	// --- Process Probe Data into Result Structure ---
	result := MediaInfoResult{}

	if probeData.Format != nil {
		result.Filename = probeData.Format.Filename
		result.Duration = probeData.Format.Duration
	}

	result.Streams = make([]StreamInfo, 0, len(probeData.Streams))
	for _, s := range probeData.Streams {
		if s == nil {
			continue
		} // Add nil check for safety
		streamInfo := StreamInfo{
			Index:      s.Index,
			CodecType:  s.CodecType,
			CodecName:  s.CodecName,
			Profile:    s.Profile,
			Width:      s.Width,
			Height:     s.Height,
			HasBFrames: s.HasBFrames,
			SampleRate: s.SampleRate,
			Channels:   s.Channels,
			Tags:       s.Tags,
		}
		result.Streams = append(result.Streams, streamInfo)
	}

	result.Frames = make([]FrameInfo, 0, len(probeData.Frames))
	for _, f := range probeData.Frames {
		if f == nil {
			continue
		} // Add nil check for safety
		frameInfo := FrameInfo{
			StreamIndex:  f.StreamIndex,
			MediaType:    f.MediaType,
			Pts:          f.Pts,
			PtsTime:      f.PtsTime,
			PictType:     f.PictType,
			Width:        f.Width,
			Height:       f.Height,
			PktPts:       f.PktPts,
			PktDts:       f.PktDts,
			SideDataList: f.SideDataList, // Include potential SEI data
		}
		// Ensure PictType, Width, Height are omitted for non-video frames if desired
		if f.MediaType != "video" {
			frameInfo.PictType = ""
			frameInfo.Width = 0
			frameInfo.Height = 0
		}

		result.Frames = append(result.Frames, frameInfo)
	}

	// --- Marshal Final Result ---
	resultJSON, err := json.MarshalIndent(result, "", "  ") // Use MarshalIndent for readability
	if err != nil {
		errMsg := fmt.Sprintf("Failed to marshal media info result to JSON: %v", err)
		log.Printf("Error: %s", errMsg)
		return mcp.NewToolResultError(errMsg), nil
	}

	log.Printf("Successfully retrieved media info (%d streams, %d frames) for: '%s'", len(result.Streams), len(result.Frames), inputPath)
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func handleTranscodePush(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	inputPath, ok := request.Params.Arguments["input_path"].(string)
	if !ok || inputPath == "" {
		return mcp.NewToolResultError("Missing or invalid 'input_path' parameter"), nil
	}
	outputURL, ok := request.Params.Arguments["output_url"].(string)
	if !ok || outputURL == "" {
		return mcp.NewToolResultError("Missing or invalid 'output_url' parameter"), nil
	}

	// --- Input Arguments ---
	// Add stream_loop=-1 for continuous looping of input (useful for some stream tests)
	// 添加 stream_loop=-1 用于输入的连续循环（对某些流测试有用）
	// Make this optional via MCP params if needed
	// 如果需要，可通过 MCP 参数使其可选
	inputArgs := ffmpeg.KwArgs{
		// "stream_loop": "-1", // Example: Add loop if needed 示例：如果需要添加循环
	}

	// --- Output Arguments ---
	outputArgs := ffmpeg.KwArgs{}

	// Codecs (from MCP params or default to copy)
	// 编解码器（来自 MCP 参数或默认为复制）
	if codecVideo, ok := request.Params.Arguments["codec_video"].(string); ok && codecVideo != "" {
		outputArgs["c:v"] = codecVideo
	} else {
		outputArgs["c:v"] = "copy"
	}

	if codecAudio, ok := request.Params.Arguments["codec_audio"].(string); ok && codecAudio != "" {
		outputArgs["c:a"] = codecAudio
	} else {
		outputArgs["c:a"] = "copy"
	}

	// Format (from MCP params or default to flv for RTMP-like push)
	// 格式（来自 MCP 参数或默认为 flv 用于类似 RTMP 的推送）
	if format, ok := request.Params.Arguments["format"].(string); ok && format != "" {
		outputArgs["f"] = format
	} else {
		// Default to flv if output looks like RTMP, otherwise let ffmpeg guess
		// 如果输出看起来像 RTMP，则默认为 flv，否则让 ffmpeg 推断
		// A more robust check might be needed 可能需要更可靠的检查
		if len(outputURL) > 7 && outputURL[:7] == "rtmp://" {
			outputArgs["f"] = "flv"
		}
	}

	// Bitrate (from MCP params)
	// 比特率（来自 MCP 参数）
	if bitrateVideo, ok := request.Params.Arguments["bitrate_video"].(string); ok && bitrateVideo != "" {
		outputArgs["b:v"] = bitrateVideo
	}

	// Add streaming-specific reconnection flags
	// 添加流式传输特定的重连标志
	// These could also be exposed as MCP parameters if needed
	// 如果需要，这些也可以作为 MCP 参数公开
	outputArgs["reconnect"] = "1"
	outputArgs["reconnect_streamed"] = "1"
	outputArgs["reconnect_delay_max"] = "2" // Or a different value? 或其他值？

	log.Printf("Attempting to transcode/push: input='%s', output='%s', input_args=%+v, output_args=%+v", inputPath, outputURL, inputArgs, outputArgs)

	var stderr bytes.Buffer
	// Apply input args, output args, and global args
	// 应用输入参数、输出参数和全局参数
	ffStream := ffmpeg.Input(inputPath, inputArgs). // Pass input args here 在此处传递输入参数
							Output(outputURL, outputArgs).
							OverWriteOutput().
		// Add -re for native frame rate input, crucial for live/streaming
		// 添加 -re 以实现原生帧率输入，对直播/流式传输至关重要
		// Keep -progress pipe:1 for potential progress parsing
		// 保留 -progress pipe:1 用于可能的进度解析
		GlobalArgs("-re", "-progress", "pipe:1")

	cmd := ffStream.Compile()

	cmd.Path = getExecutablePath("ffmpeg")
	cmd.Stderr = &stderr

	log.Printf("Running command: %v", cmd.Args)

	err := cmd.Run()

	if err != nil {
		errMsg := fmt.Sprintf("ffmpeg execution failed for transcode/push: %v", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			errMsg = fmt.Sprintf("%s\nFFmpeg Stderr:\n%s", errMsg, stderr.String())
		}
		log.Printf("Error: %s", errMsg)
		return mcp.NewToolResultError(errMsg), nil
	}

	log.Printf("Transcode/push successful: input='%s', output='%s'", inputPath, outputURL)
	return mcp.NewToolResultText(fmt.Sprintf("Successfully started transcode/push from '%s' to '%s'", inputPath, outputURL)), nil
}
