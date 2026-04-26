# shrike demo - render with `vhs docs/demo.tape` (install vhs: `brew install vhs`).
# Output: docs/demo.gif

# Settings MUST come before any other directive (Output, Env, …) — otherwise
# vhs silently ignores them and falls back to defaults (1200×600, FontSize 22).
Set Shell "bash"
Set FontSize 14
Set Width 1480
Set Height 1100
Set Padding 80
Set TypingSpeed 60ms
Set Theme "Catppuccin Mocha"

Output docs/demo.gif

# Hide personal username — replaces real usernames with "user" in the output.
Env SHRIKE_ANONYMIZE_USER "1"

# Run the locally-built binary so we get the freshest code regardless of what
# is installed system-wide. Skip the runaway detector so the list stays short
# enough that the ASCII-art header is always on screen.
Type "./shrike doctor --only zombie,herd"
Sleep 800ms
Enter

# Scan + main view render. Long pause so the logo lands on screen and the
# viewer's eye has time to read it.
Sleep 6s

# Walk through the list — slower so each step is readable.
Down
Sleep 1500ms
Down
Sleep 1500ms
Space
Sleep 1500ms
Down
Sleep 1500ms
Space
Sleep 2s

# Open info modal and let the reader actually read it.
Type "i"
Sleep 5s
Escape
Sleep 1s

# Try kill — opens confirm dialog. Hold long enough to read the prompt.
Type "k"
Sleep 3s
# Cancel
Type "n"
Sleep 1500ms

# Show help — dense, needs time.
Type "?"
Sleep 5s
Escape
Sleep 800ms

# Quit
Type "q"
Sleep 800ms
