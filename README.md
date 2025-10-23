# Memento — Shell History → Spaced Repetition


Turns your shell history into spaced‑repetition flashcards. Terminal TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).


<p align="center">
<img src="assets/logo.svg" alt="Memento logo" width="140"/>
</p>


## Why
Your future self forgets flags. Memento learns from `bash`/`zsh` history, scrubs obvious secrets, and turns tricky commands into flashcards. Review for ~5 minutes a day—inside your terminal.


## Features
-  **Automatic ingest** from `~/.zsh_history` / `~/.bash_history`
-  **Smart heuristics** find long/piped/multi‑flag commands ("tricky")
-  **Cloze cards** that hide a flag/subcommand
-  **Leitner boxes** (1→5) with sane default intervals
-  **Tags** inferred from tools (git/kubectl/ffmpeg/etc.)
-  **Local‑only storage** (JSON in XDG data dir)

## Privacy
Your history never leaves your machine. We scrub obvious tokens/emails/hex keys during ingest. Review the regexes in `ingest.go` and adjust for your environment.

## Roadmap
- [ ] Tag filters
- [ ] Multiple-Choice
- [ ] Import/export anonymized decks
