# shrike demo - render with `vhs docs/demo.tape` (install vhs: `brew install vhs`).
# Output: docs/demo.gif

# Settings MUST come before any other directive (Output, Env, …) — otherwise
# vhs silently ignores them and falls back to defaults (1200×600, FontSize 22).
Set Shell "bash"
Set FontSize 14
Set Width 1480
Set Height 720
Set Padding 80
Set TypingSpeed 60ms
Set Theme "Catppuccin Mocha"

Output docs/demo.gif

# Hide personal username — replaces real usernames with "user" in the output.
Env SHRIKE_ANONYMIZE_USER "1"

# Run the locally-built binary with a low CPU threshold so the list actually
# has findings to look at. Combined with the new scroll feature, the window
# can now stay short — the user pages through findings instead of needing a
# tall terminal.
Type "./shrike doctor --threshold 10"
Sleep 800ms
Enter

# Scan + main view render. Long pause so the logo lands on screen and the
# viewer's eye has time to read it.
Sleep 5s

# Walk through the list — slower so each step is readable.
Down
Sleep 1200ms
Down
Sleep 1200ms
Space
Sleep 1500ms
Down
Sleep 1200ms
Space
Sleep 2s

# Demonstrate the new scroll feature — page down past the visible window, the
# "↓ N more below" indicator flips, then page back up to the top.
PageDown
Sleep 2s
PageDown
Sleep 2s
PageUp
Sleep 1500ms
PageUp
Sleep 1500ms

# Open info modal and let the reader actually read it.
Type "i"
Sleep 4s
Escape
Sleep 800ms

# Try kill — opens confirm dialog. Hold long enough to read the prompt.
Type "k"
Sleep 3s
# Cancel
Type "n"
Sleep 1200ms

# Show help — dense, needs time.
Type "?"
Sleep 4s
Escape
Sleep 800ms

# Quit
Type "q"
Sleep 800ms
