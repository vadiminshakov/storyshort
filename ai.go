package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OpenAIProcessor struct {
	config *Config
}

func NewOpenAIProcessor(config *Config) *OpenAIProcessor {
	return &OpenAIProcessor{config: config}
}

type openAITranscriptionResponse struct {
	Text string `json:"text"`
}

func (p *OpenAIProcessor) ProcessAudio(audioFile, outputDir, language, model string, startTime time.Time) (summary, title, finalAudioPath string, err error) {
	apiKey := p.config.GetOpenAIAPIKey()
	if apiKey == "" {
		return "", "", "", fmt.Errorf("OpenAI API key is required")
	}

	fmt.Printf("DEBUG: Starting transcription for file: %s\n", audioFile)
	
	transcript, err := p.transcribeAudio(audioFile, language, model)
	if err != nil {
		return "", "", "", fmt.Errorf("transcription failed: %w", err)
	}

	if transcript == "" {
		return "", "", "", fmt.Errorf("empty transcript received")
	}

	summary, title, err = p.generateSummary(transcript)
	if err != nil {
		return "", "", "", fmt.Errorf("summary generation failed: %w", err)
	}

	fmt.Printf("DEBUG: Summary generation successful\n")

	sessionDir, err := createSessionDir(outputDir, title, startTime)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create session directory: %w", err)
	}

	finalAudioPath = filepath.Join(sessionDir, "recording.wav")
	if err := os.Rename(audioFile, finalAudioPath); err != nil {
		return "", "", "", fmt.Errorf("failed to move audio file: %w", err)
	}

	transcriptPath := filepath.Join(sessionDir, "transcript.txt")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0644); err != nil {
		fmt.Printf("Warning: failed to save transcript: %v\n", err)
	}

	return summary, title, finalAudioPath, nil
}

func (p *OpenAIProcessor) transcribeAudio(audioFile, language, model string) (string, error) {
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

	writer.WriteField("model", model)
	if language != "auto" {
		writer.WriteField("language", language)
	}
	writer.Close()

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.GetOpenAIAPIKey())
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

	var transcription openAITranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcription); err != nil {
		return "", err
	}

	return transcription.Text, nil
}

func (p *OpenAIProcessor) generateSummary(transcript string) (summary, title string, err error) {
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

	requestBody := map[string]any{
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
	req.Header.Set("Authorization", "Bearer "+p.config.GetOpenAIAPIKey())

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

	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", "", err
	}

	choices, ok := response["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", "", fmt.Errorf("invalid response format")
	}

	message, ok := choices[0].(map[string]any)["message"].(map[string]any)
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

func createSessionDir(outputDir, title string, startTime time.Time) (string, error) {
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

func saveSummary(title, summary string, meetingDate time.Time, sessionDir string) (string, error) {
	fileName := "summary.txt"
	filePath := filepath.Join(sessionDir, fileName)

	summary = strings.ReplaceAll(summary, "\\n", "\n")
	
	content := fmt.Sprintf("Встреча: %s\nДата: %s\n\n%s", title, meetingDate.Format("2006-01-02 15:04:05"), summary)

	return filePath, os.WriteFile(filePath, []byte(content), 0644)
}