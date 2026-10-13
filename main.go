package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Print(`Memonto — Shell History for Your Brain
Usage:
memonto ingest # parse bash/zsh history → generate/update cards
memonto review # TUI daily review (Leitner boxes)
memonto help # show this help
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
		events, err := ParseHistory()
		if err != nil {
			fatal(err)
		}
		newCards := GenerateCards(events, cards)
		cards = UpsertCards(cards, newCards)
		if err := SaveCards(cards); err != nil {
			fatal(err)
		}
		if len(newCards) > 0 {
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
