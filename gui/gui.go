package gui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
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

var (
	primaryColor     = color.NRGBA{R: 33, G: 150, B: 243, A: 255}  // Blue
	backgroundColor  = color.NRGBA{R: 250, G: 250, B: 250, A: 255} // Light Gray
	surfaceColor     = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // White
	onSurfaceColor   = color.NRGBA{R: 33, G: 33, B: 33, A: 255}    // Dark Gray
)

type materialTheme struct{}

func (m materialTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return primaryColor
	case theme.ColorNameBackground:
		return backgroundColor
	case theme.ColorNameForeground:
		return onSurfaceColor
	case theme.ColorNameButton:
		return surfaceColor
	case theme.ColorNameInputBackground:
		return surfaceColor
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (m materialTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m materialTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m materialTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 4
	default:
		return theme.DefaultTheme().Size(name)
	}
}

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
	myApp.Settings().SetTheme(&materialTheme{})
	myApp.SetIcon(ResourceIconSvg)
	
	window := myApp.NewWindow("StoryShort")
	window.SetIcon(ResourceIconSvg)
	window.Resize(fyne.NewSize(350, 650))
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

func (g *App) createCard(title string, content fyne.CanvasObject) *fyne.Container {
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	
	bg := canvas.NewRectangle(surfaceColor)
	bg.CornerRadius = 8
	bg.StrokeColor = color.NRGBA{R: 224, G: 224, B: 224, A: 255}
	bg.StrokeWidth = 1
	
	cardContent := container.NewVBox(
		titleLabel,
		content,
	)
	
	return container.NewStack(bg, container.NewBorder(nil, nil, nil, nil, 
		container.NewPadded(cardContent)))
}

func (g *App) createElevatedButton(text string, importance widget.ButtonImportance, callback func()) *widget.Button {
	btn := widget.NewButton(text, callback)
	btn.Importance = importance
	return btn
}

func (g *App) createStatChip(label, value string) *fyne.Container {
	bg := canvas.NewRectangle(color.NRGBA{R: 245, G: 245, B: 245, A: 255})
	bg.CornerRadius = 12
	
	labelWidget := widget.NewLabel(label)
	labelWidget.TextStyle = fyne.TextStyle{Bold: true}
	
	valueWidget := widget.NewLabel(value)
	valueWidget.Alignment = fyne.TextAlignCenter
	
	content := container.NewHBox(labelWidget, valueWidget)
	
	paddedContent := container.NewBorder(nil, nil, nil, nil, content)
	paddedContent.Resize(fyne.NewSize(80, 24))
	
	return container.NewStack(bg, paddedContent)
}

func (g *App) setupUI() {
	g.statusLabel = widget.NewLabel("Ready to record")
	g.statusLabel.Alignment = fyne.TextAlignCenter
	g.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	
	g.timeLabel = widget.NewLabel("00:00")
	g.sizeLabel = widget.NewLabel("0.0 MB")
	
	g.recordBtn = g.createElevatedButton("🎙️ Start Recording", widget.HighImportance, g.toggleRecording)
	
	g.tokenEntry = widget.NewPasswordEntry()
	g.tokenEntry.SetPlaceHolder("Enter OpenAI API key...")
	
	tokenLabel := widget.NewLabel("API Token")
	tokenLabel.TextStyle = fyne.TextStyle{Bold: true}
	
	saveTokenBtn := g.createElevatedButton("Save", widget.MediumImportance, g.saveToken)
	
	tokenContent := container.NewVBox(
		tokenLabel,
		g.tokenEntry,
		saveTokenBtn,
	)
	
	g.folderLabel = widget.NewLabel("No folder selected")
	g.folderLabel.Wrapping = fyne.TextWrapWord
	
	folderBtn := g.createElevatedButton("📁 Select", widget.MediumImportance, g.selectFolder)
	
	storageContent := container.NewVBox(
		widget.NewLabel("Save Location"),
		g.folderLabel,
		folderBtn,
	)
	
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
	
	optionsContent := container.NewVBox(
		widget.NewLabel("Language"),
		g.languageSelect,
		widget.NewLabel("Model"),
		g.modelSelect,
	)
	
	statsContainer := container.NewGridWithColumns(2,
		g.createStatChip("⏱", "00:00"),
		g.createStatChip("💾", "0.0 MB"),
	)

	recordingContent := container.NewVBox(
		g.statusLabel,
		statsContainer,
		g.recordBtn,
	)
	
	content := container.NewVBox(
		g.createCard("🎯 Recording", recordingContent),
		g.createCard("🔑 Auth", tokenContent),
		g.createCard("💾 Storage", storageContent),
		g.createCard("⚙️ Options", optionsContent),
	)
	
	scroll := container.NewScroll(content)
	scroll.SetMinSize(fyne.NewSize(320, 480))
	
	g.window.SetContent(scroll)
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
		
		dialog.ShowInformation("Success! 🔐", "API token has been saved!", g.window)
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
		g.showError("Authentication Required", fmt.Errorf("please enter your OpenAI API token first"))
		return
	}
	
	if err := g.recorder.InitializeAudio(); err != nil {
		g.showError("Audio Setup Failed", err)
		return
	}
	
	if err := g.recorder.StartRecording(); err != nil {
		g.showError("Recording Failed", err)
		return
	}
	
	g.isRecording = true
	g.startTime = time.Now()
	g.recordBtn.SetText("⏹️ Stop Recording")
	g.recordBtn.Importance = widget.DangerImportance
	g.statusLabel.SetText("🔴 Recording in progress...")
	
	g.ticker = time.NewTicker(time.Second)
	go g.updateStats()
}

func (g *App) stopRecording() {
	if g.ticker != nil {
		g.ticker.Stop()
	}
	
	if err := g.recorder.StopRecording(); err != nil {
		g.showError("Stop Recording Failed", err)
		return
	}
	
	g.isRecording = false
	g.recordBtn.SetText("🎙️ Start Recording")
	g.recordBtn.Importance = widget.HighImportance
	g.statusLabel.SetText("📦 Compressing & processing...")
	
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
				g.statusLabel.SetText("⚠️ No audio detected")
				dialog.ShowInformation("Notice", "No audio was detected. Please try recording again.", g.window)
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
	g.statusLabel.SetText("✅ Complete!")
	
	resultText := fmt.Sprintf("✨ Your recording has been processed!\n\n📝 Topic: %s\n\n💾 Files saved to:\n%s", 
		title, sessionDir)
	
	dialog.ShowInformation("Success! 🎉", resultText, g.window)
	
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