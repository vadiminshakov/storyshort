# StoryShort

A simple GUI application for recording audio and generating AI-powered meeting summaries using OpenAI's transcription and chat APIs.

## Installation

### Quick Install (Recommended)

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/vadiminshakov/storyshort/main/install.sh | bash
```

### Manual Installation

1. Download the appropriate binary for your platform from [Releases](https://github.com/vadiminshakov/storyshort/releases)
2. Extract the archive: `tar -xzf storyshort_*.tar.gz`
3. Move to system path: `sudo mv storyshort /usr/local/bin/`

## Usage

1. **Start the application:**

2. **Configure OpenAI API:**
   - Enter your OpenAI API key in the settings
   - Choose your preferred language and model
   - Select save location for recordings

3. **Record and Process:**
   - Click "Start Recording" to begin
   - Click "Stop Recording" when finished
   - The app will automatically transcribe and generate a summary

## Requirements

- **OpenAI API Key** - Required for transcription and summary generation
- **Audio Tools** - The app will automatically install `sox` or `ffmpeg` if needed


## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.