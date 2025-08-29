package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionState int

const (
	configView sessionState = iota
	setupView
	recordingView
	processingView
	completedView
)

type model struct {
	state      sessionState
	recorder   *AudioRecorder
	progress   progress.Model
	spinner    spinner.Model
	width      int
	height     int
	err        error
	audioFile  string
	summaryFile string
	title      string
	startTime  time.Time
	noAudioWarning bool
	config     *Config
	tokenInput string
}

type recordingStartMsg struct{}
type recordingStoppedMsg struct{ audioFile string }
type processingCompleteMsg struct{ summaryFile, title string }
type noAudioDataMsg struct{}
type configLoadedMsg struct{ config *Config }
type tokenSavedMsg struct{}
type errorMsg struct{ err error }

var (
	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2)

	statusStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true)

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF5F87")).
		Bold(true)

	infoStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C7C7C"))

	recordingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Render
)

func initialModel() model {
	p := progress.New(progress.WithDefaultGradient())
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return model{
		state:    configView,
		recorder: &AudioRecorder{outputDir: "recordings"},
		progress: p,
		spinner:  s,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadConfig,
	)
}

func (m model) loadConfig() tea.Msg {
	config, err := loadConfig()
	if err != nil {
		return errorMsg{err}
	}
	return configLoadedMsg{config}
}

func (m model) saveToken() tea.Msg {
	m.config.OpenAIAPIKey = m.tokenInput
	if err := saveConfig(m.config); err != nil {
		return errorMsg{err}
	}
	return tokenSavedMsg{}
}

func (m model) initializeAndStartRecording() tea.Msg {
	if err := m.recorder.initializeAudio(); err != nil {
		return errorMsg{err}
	}
	
	// Автоматически начинаем запись после инициализации
	if err := m.recorder.StartRecording(); err != nil {
		return errorMsg{err}
	}
	
	return recordingStartMsg{}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case configView:
			// Только ввод токена
			switch msg.Type {
			case tea.KeyEnter:
				if len(m.tokenInput) > 0 {
					return m, m.saveToken
				}
			case tea.KeyEsc, tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyBackspace:
				if len(m.tokenInput) > 0 {
					m.tokenInput = m.tokenInput[:len(m.tokenInput)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.tokenInput += string(msg.Runes)
				}
			}
		case setupView:
			// В setup экране только выход
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
				return m, tea.Quit
			}
		case recordingView:
			// В записи остановка через Esc, q, Ctrl+C
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
				return m, m.stopRecording
			}
		case processingView:
			// Во время обработки можно только принудительно выйти
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		case completedView:
			// После завершения выход или новая запись
			if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "esc" {
				return m, tea.Quit
			}
			if msg.String() == "enter" || msg.String() == "r" {
				// Начать новую запись
				m.state = setupView
				m.noAudioWarning = false
				m.err = nil
				return m, m.initializeAndStartRecording
			}
		}

	case recordingStartMsg:
		m.state = recordingView
		m.startTime = time.Now()
		return m, m.tickRecording()

	case recordingStoppedMsg:
		m.state = processingView
		m.audioFile = msg.audioFile
		return m, tea.Batch(
			m.spinner.Tick,
			m.processWithOpenAI,
		)

	case processingCompleteMsg:
		m.state = completedView
		m.summaryFile = msg.summaryFile
		m.title = msg.title
		return m, nil

	case configLoadedMsg:
		m.config = msg.config
		if m.config.HasValidToken() {
			// Токен есть, переходим к записи
			m.state = setupView
			return m, m.initializeAndStartRecording
		} else {
			// Токена нет, остаемся на экране конфигурации
			return m, nil
		}

	case tokenSavedMsg:
		// Токен сохранен, переходим к записи
		m.state = setupView
		return m, m.initializeAndStartRecording

	case noAudioDataMsg:
		m.state = setupView
		m.noAudioWarning = true
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			// Через 3 секунды автоматически перезапускаем запись
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
		})

	case errorMsg:
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.state == processingView {
			// Продолжаем тикать спиннер во время обработки
			return m, tea.Batch(cmd, m.spinner.Tick)
		}
		return m, cmd

	default:
		if m.state == recordingView {
			return m, m.tickRecording()
		}
		if m.state == processingView {
			return m, m.spinner.Tick
		}
	}

	return m, nil
}


func (m model) stopRecording() tea.Msg {
	if err := m.recorder.StopRecording(); err != nil {
		return errorMsg{err}
	}

	// Сначала сохраняем во временную папку
	tempDir := "temp_recording"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return errorMsg{err}
	}

	audioFile, err := m.recorder.SaveAudio(tempDir)
	if err != nil {
		if err.Error() == "no_audio_data" {
			return noAudioDataMsg{}
		}
		return errorMsg{err}
	}

	return recordingStoppedMsg{audioFile}
}

func (m model) processWithOpenAI() tea.Msg {
	summary, title, finalAudioPath, err := processWithOpenAI(m.audioFile, m.config.OpenAIAPIKey, m.recorder.outputDir, m.startTime)
	if err != nil {
		return errorMsg{err}
	}

	// Получаем sessionDir из пути к финальному аудиофайлу
	sessionDir := filepath.Dir(finalAudioPath)
	summaryFile, err := saveSummary(title, summary, m.startTime, sessionDir)
	if err != nil {
		return errorMsg{err}
	}

	// Удаляем временную папку
	os.RemoveAll("temp_recording")

	// Обновляем путь к аудиофайлу для отображения в интерфейсе
	m.audioFile = finalAudioPath

	return processingCompleteMsg{summaryFile, title}
}

func (m model) tickRecording() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return t
	})
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render("❌ Ошибка: " + m.err.Error())
	}

	switch m.state {
	case configView:
		return m.configView()
	case setupView:
		return m.setupView()
	case recordingView:
		return m.recordingView()
	case processingView:
		return m.processingView()
	case completedView:
		return m.completedView()
	default:
		return "Неизвестное состояние"
	}
}

func (m model) configView() string {
	title := titleStyle.Render("🔑 OpenAI API Токен")
	
	if m.config == nil {
		content := []string{
			title,
			"",
			fmt.Sprintf("%s Загрузка конфигурации...", m.spinner.View()),
		}
		return headerStyle.Render(strings.Join(content, "\n"))
	}
	
	// Всегда показываем ввод токена
	maskedToken := strings.Repeat("*", len(m.tokenInput))
	if len(m.tokenInput) == 0 {
		maskedToken = infoStyle.Render("введите токен...")
	}
	
	content := []string{
		title,
		"",
		"Для работы необходим OpenAI API токен:",
		"",
		fmt.Sprintf("Токен: %s", maskedToken),
		"",
		infoStyle.Render("Получить токен: https://platform.openai.com/api-keys"),
		infoStyle.Render("Сохраняется в: ~/.shortstory/config.json"),
		"",
		helpStyle("Enter - сохранить и продолжить • Esc/Ctrl+C - выйти"),
	}
	
	return headerStyle.Render(strings.Join(content, "\n"))
}

func (m model) setupView() string {
	title := titleStyle.Render("🎙️ StoryShort Recorder")
	
	content := []string{
		title,
		"",
	}
	
	if m.noAudioWarning {
		content = append(content, infoStyle.Render("ℹ️  Аудиоданные не были записаны."))
		content = append(content, infoStyle.Render("    Автоматический перезапуск через несколько секунд..."))
		content = append(content, "")
	} else {
		content = append(content, "🚀 Подготовка к записи...")
		content = append(content, "")
		content = append(content, "Система автоматически:")
		content = append(content, "• Инициализирует аудиосистему")
		content = append(content, "• Начнёт запись сразу после готовности")
		content = append(content, "• Обработает результат с OpenAI")
		content = append(content, "")
		content = append(content, statusStyle.Render("⚡ Запуск..."))
		content = append(content, "")
	}
	
	content = append(content, helpStyle("Esc/q/Ctrl+C для выхода"))

	return headerStyle.Render(strings.Join(content, "\n"))
}

func (m model) recordingView() string {
	elapsed := time.Since(m.startTime)
	duration := fmt.Sprintf("%02d:%02d", 
		int(elapsed.Minutes()), 
		int(elapsed.Seconds())%60)

	audioSize := float64(len(m.recorder.audioData)) / 1024 / 1024 // MB

	title := titleStyle.Render("🎙️ Запись в процессе")
	
	content := []string{
		title,
		"",
		recordingStyle.Render("🔴 REC"),
		"",
		fmt.Sprintf("⏱️  Длительность: %s", duration),
		fmt.Sprintf("💾 Размер файла: %.1f MB", audioSize),
		"",
		"Говорите в микрофон или включите аудио встречи...",
		"",
		helpStyle("Esc/q/Ctrl+C для остановки записи"),
	}

	return headerStyle.Render(strings.Join(content, "\n"))
}

func (m model) processingView() string {
	title := titleStyle.Render("🤖 Обработка с OpenAI")
	
	content := []string{
		title,
		"",
		fmt.Sprintf("%s Обрабатываем запись...", m.spinner.View()),
		"",
		"📄 Транскрибируем аудио с помощью Whisper",
		"🧠 Генерируем саммари с помощью GPT-4",
		"💾 Сохраняем результаты в файл",
		"",
		infoStyle.Render("Это может занять несколько минут в зависимости от длины записи"),
		"",
		helpStyle("Ctrl+C - принудительный выход"),
	}

	return headerStyle.Render(strings.Join(content, "\n"))
}

func (m model) completedView() string {
	title := titleStyle.Render("✨ Готово!")
	
	// Получаем имя папки сессии
	sessionName := filepath.Base(filepath.Dir(m.audioFile))
	
	content := []string{
		title,
		"",
		statusStyle.Render("🎉 Запись успешно обработана!"),
		"",
		fmt.Sprintf("📁 Папка сессии: %s", sessionName),
		fmt.Sprintf("📋 Тема встречи: %s", m.title),
		"",
		"Сохранены файлы:",
		"  🎵 recording.wav - аудиозапись",
		"  📄 transcript.txt - транскрипция",
		"  📝 summary.txt - саммари встречи",
		"",
		fmt.Sprintf("Путь: %s", infoStyle.Render("recordings/"+sessionName+"/")),
		"",
		helpStyle("Esc/q/Ctrl+C для выхода • Enter/r для новой записи"),
	}

	return headerStyle.Render(strings.Join(content, "\n"))
}

func runUI() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}