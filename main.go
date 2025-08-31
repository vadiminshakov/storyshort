package main

import (
	"log"

	"github.com/vadiminshakov/storyshort/gui"
)


func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatal("Config error:", err)
	}
	
	recorder := &AudioRecorder{}
	aiProcessor := NewOpenAIProcessor(config)
	
	app := gui.NewApp(recorder, config, aiProcessor, saveSummary)
	app.Run()
}
