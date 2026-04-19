# shrike demo - render with `vhs docs/demo.tape` (install vhs: `brew install vhs`).
# Output: docs/demo.gif

Output docs/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1280
Set Height 720
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# Assume `shrike` is on PATH (or run from repo root with `./shrike`).
Type "shrike doctor"
Enter
Sleep 2s

# Walk through the list
Down
Sleep 500ms
Down
Space
Sleep 500ms
Down
Space
Sleep 1s

# Open info modal, then close
Type "i"
Sleep 2s
Escape
Sleep 500ms

# Try kill — opens confirm dialog
Type "k"
Sleep 2s

# Cancel
Type "n"
Sleep 500ms

# Show help
Type "?"
Sleep 3s
Escape
Sleep 500ms

# Quit
Type "q"
