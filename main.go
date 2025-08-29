package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/youpy/go-wav"
)

const (
	sampleRate      = 44100
	channels        = 1
	bitsPerSample   = 16
	chunkDuration   = 30 * time.Second
)

type AudioRecorder struct {
	isRecording    bool
	audioData      []byte
	startTime      time.Time
	outputDir      string
	sessionDir     string
	recordingDone  chan struct{}
	cmd            *exec.Cmd
	statusUpdate   chan string
}

type OpenAIResponse struct {
	Text string `json:"text"`
}

type OpenAITranscriptionResponse struct {
	Text string `json:"text"`
}

func main() {
	if err := os.MkdirAll("recordings", 0755); err != nil {
		log.Fatal("Failed to create output directory:", err)
	}

	if err := runUI(); err != nil {
		log.Fatal("UI error:", err)
	}
}

func (ar *AudioRecorder) initializeAudio() error {
	if isAudioToolAvailable() {
		return nil
	}
	
	return installAudioTool()
}

func (ar *AudioRecorder) StartRecording() error {
	ar.startTime = time.Now()
	ar.isRecording = true
	ar.recordingDone = make(chan struct{})
	ar.audioData = make([]byte, 0)
	ar.statusUpdate = make(chan string, 10)

	go ar.recordAudio()
	return nil
}

func (ar *AudioRecorder) StopRecording() error {
	ar.isRecording = false
	if ar.cmd != nil && ar.cmd.Process != nil {
		ar.cmd.Process.Kill()
	}
	if ar.recordingDone != nil {
		<-ar.recordingDone
	}
	return nil
}

func (ar *AudioRecorder) recordAudio() {
	defer close(ar.recordingDone)

	var cmd *exec.Cmd
	
	if _, err := exec.LookPath("sox"); err == nil {
		cmd = exec.Command("sox", "-d", "-t", "raw", "-b", "16", "-e", "signed-integer", "-r", "44100", "-c", "1", "-")
	} else if _, err := exec.LookPath("rec"); err == nil {
		cmd = exec.Command("rec", "-t", "raw", "-b", "16", "-e", "signed-integer", "-r", "44100", "-c", "1", "-")
	} else {
		log.Println("Warning: sox/rec not found, using ffmpeg fallback")
		cmd = exec.Command("ffmpeg", "-f", "avfoundation", "-i", ":0", "-ar", "44100", "-ac", "1", "-f", "s16le", "-")
	}

	ar.cmd = cmd
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create stdout pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start recording command: %v", err)
		return
	}

	buffer := make([]byte, 4096)
	for ar.isRecording {
		n, err := stdout.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading audio data: %v", err)
			}
			break
		}
		if n > 0 {
			ar.audioData = append(ar.audioData, buffer[:n]...)
		}
	}

	stdout.Close()
	cmd.Wait()
}

func (ar *AudioRecorder) SaveAudio(sessionDir string) (string, error) {
	if len(ar.audioData) == 0 {
		return "", fmt.Errorf("no_audio_data")
	}

	fileName := "recording.wav"
	filePath := filepath.Join(sessionDir, fileName)

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := wav.NewWriter(file, uint32(len(ar.audioData)/2), channels, sampleRate, bitsPerSample)
	
	samples := make([]wav.Sample, len(ar.audioData)/2)
	for i := 0; i < len(samples); i++ {
		if i*2+1 < len(ar.audioData) {
			sample := int16(ar.audioData[i*2]) | int16(ar.audioData[i*2+1])<<8
			samples[i].Values[0] = int(sample)
		}
	}

	if err := writer.WriteSamples(samples); err != nil {
		return "", err
	}

	return filePath, nil
}

func createSessionDir(outputDir, title string, startTime time.Time) (string, error) {
	// Очищаем заголовок от недопустимых символов для имени папки
	cleanTitle := strings.ReplaceAll(title, "/", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "\\", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, ":", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "*", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "?", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "|", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "<", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, ">", "_")
	cleanTitle = strings.ReplaceAll(cleanTitle, "\"", "_")
	
	sessionName := fmt.Sprintf("%s_%s", cleanTitle, startTime.Format("2006-01-02_15-04-05"))
	sessionDir := filepath.Join(outputDir, sessionName)
	
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", err
	}
	
	return sessionDir, nil
}

func processWithOpenAI(audioFile, apiKey string, outputDir string, startTime time.Time) (summary, title, finalAudioPath string, err error) {
	if apiKey == "" {
		return "", "", "", fmt.Errorf("OpenAI API key is required")
	}

	// Логируем начало транскрипции
	fmt.Printf("DEBUG: Starting transcription for file: %s\n", audioFile)
	
	transcript, err := transcribeAudio(audioFile, apiKey)
	if err != nil {
		return "", "", "", fmt.Errorf("transcription failed: %w", err)
	}

	if transcript == "" {
		return "", "", "", fmt.Errorf("empty transcript received")
	}

	// Генерируем саммари для получения заголовка
	summary, title, err = generateSummary(transcript, apiKey)
	if err != nil {
		return "", "", "", fmt.Errorf("summary generation failed: %w", err)
	}

	// Логируем успешную генерацию саммари
	fmt.Printf("DEBUG: Summary generation successful\n")

	// Теперь создаем папку с правильным именем
	sessionDir, err := createSessionDir(outputDir, title, startTime)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create session directory: %w", err)
	}

	// Перемещаем аудиофайл в новую папку
	finalAudioPath = filepath.Join(sessionDir, "recording.wav")
	if err := os.Rename(audioFile, finalAudioPath); err != nil {
		return "", "", "", fmt.Errorf("failed to move audio file: %w", err)
	}

	// Сохраняем транскрипцию в новую папку
	transcriptPath := filepath.Join(sessionDir, "transcript.txt")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0644); err != nil {
		fmt.Printf("Warning: failed to save transcript: %v\n", err)
	}

	return summary, title, finalAudioPath, nil
}

func transcribeAudio(audioFile, apiKey string) (string, error) {
	file, err := os.Open(audioFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", filepath.Base(audioFile))
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}

	writer.WriteField("model", "whisper-1")
	writer.WriteField("language", "ru")
	writer.Close()

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	var transcription OpenAITranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcription); err != nil {
		return "", err
	}

	return transcription.Text, nil
}

func generateSummary(transcript, apiKey string) (summary, title string, err error) {
	prompt := fmt.Sprintf(`Проанализируй следующую транскрипцию встречи и выдели:
1. Главную тему/идею встречи (для названия файла)
2. Ключевые тезисы и решения

Транскрипция:
%s

Ответ должен быть в формате JSON:
{
  "title": "краткое название главной темы встречи",
  "summary": "подробные ключевые тезисы и решения с переносами строк (\\n) для лучшей читаемости"
}`, transcript)

	requestBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 1000,
		"temperature": 0.3,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", "", err
	}

	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", "", fmt.Errorf("invalid response format")
	}

	message, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("invalid response format")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", "", fmt.Errorf("invalid response format")
	}

	var result struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return content, "meeting_summary", nil
	}

	return result.Summary, result.Title, nil
}

func saveSummary(title, summary string, meetingDate time.Time, sessionDir string) (string, error) {
	fileName := "summary.txt"
	filePath := filepath.Join(sessionDir, fileName)

	// Заменяем экранированные переносы строк на реальные
	summary = strings.ReplaceAll(summary, "\\n", "\n")
	
	content := fmt.Sprintf("Встреча: %s\nДата: %s\n\n%s", title, meetingDate.Format("2006-01-02 15:04:05"), summary)

	return filePath, os.WriteFile(filePath, []byte(content), 0644)
}

func showProgressIndicator(done <-chan bool) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	
	for {
		select {
		case <-done:
			fmt.Print("\r\033[K") // Clear the line
			return
		case <-ticker.C:
			fmt.Printf("\r🔧 %s Инициализация аудиосистемы... ", spinner[i])
			i = (i + 1) % len(spinner)
		}
	}
}

func isAudioToolAvailable() bool {
	tools := []string{"sox", "rec", "ffmpeg"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err == nil {
			return true
		}
	}
	return false
}

func installAudioTool() error {
	switch runtime.GOOS {
	case "darwin":
		if !isBrewInstalled() {
			if err := installBrew(); err != nil {
				return fmt.Errorf("failed to install Homebrew: %w", err)
			}
		}
		
		if err := installWithCommand("brew", "install", "sox"); err != nil {
			log.Println("Failed to install sox, trying ffmpeg...")
			return installWithCommand("brew", "install", "ffmpeg")
		}
		return nil
		
	case "linux":
		if isCommandAvailable("apt-get") {
			if err := installWithCommand("sudo", "apt-get", "update"); err != nil {
				log.Printf("Warning: apt-get update failed: %v", err)
			}
			if err := installWithCommand("sudo", "apt-get", "install", "-y", "sox"); err != nil {
				log.Println("Failed to install sox, trying ffmpeg...")
				return installWithCommand("sudo", "apt-get", "install", "-y", "ffmpeg")
			}
			return nil
		} else if isCommandAvailable("yum") {
			if err := installWithCommand("sudo", "yum", "install", "-y", "sox"); err != nil {
				log.Println("Failed to install sox, trying ffmpeg...")
				return installWithCommand("sudo", "yum", "install", "-y", "ffmpeg")
			}
			return nil
		} else if isCommandAvailable("dnf") {
			if err := installWithCommand("sudo", "dnf", "install", "-y", "sox"); err != nil {
				log.Println("Failed to install sox, trying ffmpeg...")
				return installWithCommand("sudo", "dnf", "install", "-y", "ffmpeg")
			}
			return nil
		}
		return fmt.Errorf("unsupported Linux distribution")
		
	case "windows":
		return fmt.Errorf("automatic installation on Windows not supported. Please install sox or ffmpeg manually")
		
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func installWithCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isBrewInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func installBrew() error {
	cmd := exec.Command("/bin/bash", "-c", `$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)`)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

