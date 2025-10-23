package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Println(`Memento — Shell History for Your Brain
Usage:
memento ingest # parse bash/zsh history → generate/update cards
memento review # TUI daily review (Leitner boxes)
memento help # show this help
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	sub := os.Args[1]
	switch sub {
	case "ingest":
		cards, err := LoadCards()
		if err != nil {
			fatal(err)
		}
		events := ParseHistory()
		newCards := GenerateCards(events, cards)
		if len(newCards) > 0 {
			cards = UpsertCards(cards, newCards)
			if err := SaveCards(cards); err != nil {
				fatal(err)
			}
			fmt.Printf("Ingested %d new cards. Total: %d\n", len(newCards), len(cards))
		} else {
			fmt.Println("No new tricky commands found. You're a wizard.")
		}
	case "review":
		cards, err := LoadCards()
		if err != nil {
			fatal(err)
		}
		if err := RunTUI(cards); err != nil {
			fatal(err)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Println("Unknown command:", sub)
		usage()
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
