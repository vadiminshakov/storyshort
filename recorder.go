package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/youpy/go-wav"
)

const (
	sampleRate      = 44100
	channels        = 1
	bitsPerSample   = 16
)

type AudioRecorder struct {
	isRecording    bool
	audioData      []byte
	startTime      time.Time
	recordingDone  chan struct{}
	cmd            *exec.Cmd
	statusUpdate   chan string
}

func (ar *AudioRecorder) InitializeAudio() error {
	if isAudioToolAvailable() {
		return nil
	}
	
	return installAudioTool()
}

func (ar *AudioRecorder) GetAudioData() []byte {
	return ar.audioData
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
			return fmt.Errorf("Homebrew is required but not installed. Please install Homebrew first: https://brew.sh")
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
	// Try standard locations for Homebrew
	brewPaths := []string{
		"/usr/local/bin/brew",     // Intel Macs
		"/opt/homebrew/bin/brew",  // Apple Silicon Macs
	}
	
	for _, path := range brewPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	
	// Fallback to PATH lookup
	_, err := exec.LookPath("brew")
	return err == nil
}


func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}