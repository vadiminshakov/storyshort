package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type AudioRecorder interface {
	StartRecording() error
	StopRecording() error
	SaveAudio(sessionDir string) (string, error)
	GetAudioData() []byte
	InitializeAudio() error
}

type Config interface {
	HasValidToken() bool
	GetOpenAIAPIKey() string
	GetSaveLocation() string
	GetLanguage() string
	GetModel() string
	SetOpenAIAPIKey(key string)
	SetSaveLocation(location string)
	SetLanguage(language string)
	SetModel(model string)
	Save() error
}

type AIProcessor interface {
	ProcessAudio(audioFile, outputDir, language, model string, startTime time.Time) (summary, title, finalAudioPath string, err error)
}

type SaveSummaryFunc func(title, summary string, meetingDate time.Time, sessionDir string) (string, error)

type App struct {
	app             fyne.App
	window          fyne.Window
	recorder        AudioRecorder
	config          Config
	aiProcessor     AIProcessor
	recordBtn       *widget.Button
	statusLabel     *widget.Label
	timeLabel       *widget.Label
	sizeLabel       *widget.Label
	tokenEntry      *widget.Entry
	folderLabel     *widget.Label
	languageSelect  *widget.Select
	modelSelect     *widget.Select
	startTime       time.Time
	ticker          *time.Ticker
	isRecording     bool
	saveSummaryFunc SaveSummaryFunc
}

func NewApp(recorder AudioRecorder, config Config, aiProcessor AIProcessor, saveSummaryFunc SaveSummaryFunc) *App {
	myApp := app.New()
	myApp.SetIcon(ResourceIconSvg)
	
	window := myApp.NewWindow("StoryShort Recorder")
	window.SetIcon(ResourceIconSvg)
	window.Resize(fyne.NewSize(320, 500))
	window.CenterOnScreen()
	
	return &App{
		app:             myApp,
		window:          window,
		recorder:        recorder,
		config:          config,
		aiProcessor:     aiProcessor,
		saveSummaryFunc: saveSummaryFunc,
	}
}

func (g *App) setupUI() {
	g.statusLabel = widget.NewLabel("Ready to record")
	g.timeLabel = widget.NewLabel("00:00")
	g.sizeLabel = widget.NewLabel("0.0 MB")
	
	g.recordBtn = widget.NewButton("🎙️ Start Recording", g.toggleRecording)
	g.recordBtn.Importance = widget.HighImportance
	
	g.tokenEntry = widget.NewPasswordEntry()
	g.tokenEntry.SetPlaceHolder("Enter OpenAI API key...")
	
	tokenLabel := widget.NewLabel("OpenAI API Token:")
	saveTokenBtn := widget.NewButton("Save Token", g.saveToken)
	
	g.folderLabel = widget.NewLabel("No folder selected")
	folderBtn := widget.NewButton("Select Folder", g.selectFolder)
	
	languages := []string{"auto", "en", "ru", "es", "fr", "de", "it", "pt", "zh", "ja", "ko"}
	g.languageSelect = widget.NewSelect(languages, g.onLanguageChanged)
	g.languageSelect.SetSelected(g.config.GetLanguage())
	
	models := []string{
		"whisper-1 (standard model)",
		"gpt-4o-transcribe (improved accuracy)",
		"gpt-4o-mini-transcribe (fast & efficient)",
	}
	g.modelSelect = widget.NewSelect(models, g.onModelChanged)
	
	currentModel := g.config.GetModel()
	switch currentModel {
	case "whisper-1":
		g.modelSelect.SetSelected("whisper-1 (standard model)")
	case "gpt-4o-transcribe":
		g.modelSelect.SetSelected("gpt-4o-transcribe (improved accuracy)")
	case "gpt-4o-mini-transcribe":
		g.modelSelect.SetSelected("gpt-4o-mini-transcribe (fast & efficient)")
	default:
		g.modelSelect.SetSelected("whisper-1 (standard model)")
	}
	
	statsContainer := container.NewGridWithColumns(2,
		widget.NewLabel("Time:"), g.timeLabel,
		widget.NewLabel("Size:"), g.sizeLabel,
	)
	
	content := container.NewVBox(
		widget.NewCard("Settings", "", 
			container.NewVBox(
				tokenLabel,
				g.tokenEntry,
				saveTokenBtn,
				widget.NewSeparator(),
				widget.NewLabel("Save Location:"),
				g.folderLabel,
				folderBtn,
				widget.NewSeparator(),
				widget.NewLabel("Language:"),
				g.languageSelect,
				widget.NewSeparator(),
				widget.NewLabel("Model:"),
				g.modelSelect,
			),
		),
		widget.NewSeparator(),
		widget.NewCard("Recording", "",
			container.NewVBox(
				g.statusLabel,
				statsContainer,
				g.recordBtn,
			),
		),
	)
	
	g.window.SetContent(content)
}

func (g *App) Run() {
	g.setupUI()
	
	if g.config.HasValidToken() {
		g.tokenEntry.SetText(strings.Repeat("*", 20))
	}
	
	g.updateFolderDisplay()
	
	g.window.ShowAndRun()
}

func (g *App) saveToken() {
	token := g.tokenEntry.Text
	if len(token) == 0 {
		g.showError("Error", fmt.Errorf("enter token"))
		return
	}
	
	if !strings.Contains(token, "*") {
		g.config.SetOpenAIAPIKey(token)
		if err := g.config.Save(); err != nil {
			g.showError("Save Error", err)
			return
		}
		
		dialog.ShowInformation("Success", "Token saved!", g.window)
		g.tokenEntry.SetText(strings.Repeat("*", 20))
	}
}

func (g *App) toggleRecording() {
	if !g.isRecording {
		g.startRecording()
	} else {
		g.stopRecording()
	}
}

func (g *App) startRecording() {
	if !g.config.HasValidToken() {
		g.showError("Error", fmt.Errorf("please enter OpenAI API token first"))
		return
	}
	
	if err := g.recorder.InitializeAudio(); err != nil {
		g.showError("Audio Initialization Error", err)
		return
	}
	
	if err := g.recorder.StartRecording(); err != nil {
		g.showError("Recording Start Error", err)
		return
	}
	
	g.isRecording = true
	g.startTime = time.Now()
	g.recordBtn.SetText("⏹️ Stop Recording")
	g.recordBtn.Importance = widget.DangerImportance
	g.statusLabel.SetText("🔴 Recording...")
	
	g.ticker = time.NewTicker(time.Second)
	go g.updateStats()
}

func (g *App) stopRecording() {
	if g.ticker != nil {
		g.ticker.Stop()
	}
	
	if err := g.recorder.StopRecording(); err != nil {
		g.showError("Recording Stop Error", err)
		return
	}
	
	g.isRecording = false
	g.recordBtn.SetText("🎙️ Начать запись")
	g.recordBtn.Importance = widget.HighImportance
	g.statusLabel.SetText("⏳ Processing...")
	
	go g.processRecording()
}

func (g *App) updateStats() {
	for range g.ticker.C {
		if !g.isRecording {
			break
		}
		
		elapsed := time.Since(g.startTime)
		duration := fmt.Sprintf("%02d:%02d", 
			int(elapsed.Minutes()), 
			int(elapsed.Seconds())%60)
		
		audioSize := float64(len(g.recorder.GetAudioData())) / 1024 / 1024
		
		fyne.Do(func() {
			g.timeLabel.SetText(duration)
			g.sizeLabel.SetText(fmt.Sprintf("%.1f MB", audioSize))
		})
	}
}

func (g *App) processRecording() {
	tempDir := filepath.Join(os.TempDir(), "temp_recording")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		fyne.Do(func() {
			g.showError("Temp Directory Creation Error", err)
		})
		return
	}
	
	audioFile, err := g.recorder.SaveAudio(tempDir)
	if err != nil {
		if err.Error() == "no_audio_data" {
			fyne.Do(func() {
				g.statusLabel.SetText("⚠️ No audio data. Try again.")
			})
			return
		}
		fyne.Do(func() {
			g.showError("Audio Save Error", err)
		})
		return
	}
	
	summary, title, finalAudioPath, err := g.aiProcessor.ProcessAudio(audioFile, g.config.GetSaveLocation(), g.config.GetLanguage(), g.config.GetModel(), g.startTime)
	if err != nil {
		fyne.Do(func() {
			g.showError("OpenAI Processing Error", err)
		})
		return
	}
	
	sessionDir := filepath.Dir(finalAudioPath)
	summaryFile, err := g.saveSummaryFunc(title, summary, g.startTime, sessionDir)
	if err != nil {
		fyne.Do(func() {
			g.showError("Summary Save Error", err)
		})
		return
	}
	
	os.RemoveAll(tempDir)
	
	fyne.Do(func() {
		g.showResults(title, summaryFile, sessionDir)
	})
}

func (g *App) showResults(title, _ string, sessionDir string) {
	g.statusLabel.SetText("✅ Processing completed")
	
	resultText := fmt.Sprintf("Recording processed!\n\nTopic: %s\n\nFiles saved to:\n%s", 
		title, sessionDir)
	
	dialog.ShowInformation("Done!", resultText, g.window)
	
	g.timeLabel.SetText("00:00")
	g.sizeLabel.SetText("0.0 MB")
	g.statusLabel.SetText("Ready to record")
}

func (g *App) selectFolder() {
	dialog.ShowFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			g.showError("Folder Selection Error", err)
			return
		}
		if folder == nil {
			return
		}
		
		folderPath := folder.Path()
		g.config.SetSaveLocation(folderPath)
		
		if err := g.config.Save(); err != nil {
			g.showError("Settings Save Error", err)
			return
		}
		
		g.updateFolderDisplay()
	}, g.window)
}

func (g *App) updateFolderDisplay() {
	saveLocation := g.config.GetSaveLocation()
	if saveLocation == "" {
		g.folderLabel.SetText("No folder selected")
	} else {
		parts := strings.Split(saveLocation, string(filepath.Separator))
		if len(parts) >= 2 {
			displayPath := ".../" + strings.Join(parts[len(parts)-2:], "/")
			g.folderLabel.SetText(displayPath)
		} else {
			g.folderLabel.SetText(saveLocation)
		}
	}
}

func (g *App) onLanguageChanged(language string) {
	g.config.SetLanguage(language)
	if err := g.config.Save(); err != nil {
		g.showError("Language Save Error", err)
	}
}

func (g *App) onModelChanged(modelWithDescription string) {
	var model string
	switch modelWithDescription {
	case "whisper-1 (standard model)":
		model = "whisper-1"
	case "gpt-4o-transcribe (improved accuracy)":
		model = "gpt-4o-transcribe"
	case "gpt-4o-mini-transcribe (fast & efficient)":
		model = "gpt-4o-mini-transcribe"
	default:
		model = "whisper-1"
	}
	
	g.config.SetModel(model)
	if err := g.config.Save(); err != nil {
		g.showError("Model Save Error", err)
	}
}

func (g *App) showError(title string, err error) {
	dialog.ShowError(fmt.Errorf("%s: %v", title, err), g.window)
	g.statusLabel.SetText("❌ Error")
	
	if g.isRecording {
		g.isRecording = false
		g.recordBtn.SetText("🎙️ Start Recording")
		g.recordBtn.Importance = widget.HighImportance
		if g.ticker != nil {
			g.ticker.Stop()
		}
	}
}