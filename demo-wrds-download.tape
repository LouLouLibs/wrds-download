# VHS tape for recording wrds-dl TUI demo GIF.
# Usage: vhs demo-wrds-download.tape
#
# Prerequisites:
#   - wrds-dl binary built and in PATH (or ./go/wrds-dl)
#   - WRDS credentials saved (~/.config/wrds-dl/credentials)
#     or exported as PGUSER / PGPASSWORD environment variables
#   - Active network connection to WRDS (Duo 2FA approved)

Output demo-wrds-download-raw.gif

Set FontSize 18
Set Width 1400
Set Height 700
Set Padding 20
Set Theme "GruvboxDarkHard"

Set TypingSpeed 80ms

Set Shell "bash"

Hide
    Type 'export PS1="> "'
    Enter
    Type "clear"
    Enter
Show

# ─── 1. LAUNCH THE TUI ───
Type "./wrds-dl tui"
Enter

# Wait for login screen to render
Sleep 2s

# ─── 2. LOGIN WITH SAVED CREDENTIALS ───
# The login form shows "Login as <user> [enter]" when credentials exist.
# Press Enter to connect with saved credentials.
Enter

# Wait for connection + Duo 2FA + schemas to load
Sleep 2s

# ─── 3. BROWSE SCHEMAS ───
# Schemas pane is focused by default. Navigate to find "crsp".
Sleep 500ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 1s

# Use "/" to filter schemas
Type "/"
Sleep 300ms
Type "crsp"
Sleep 1s
Enter
Sleep 1s

# ─── 4. DRILL INTO SCHEMA ───
# Press right arrow to select the schema and load its tables.
Right
Sleep 2s

# ─── 5. BROWSE TABLES ───
# Tables pane is now focused. Filter for "msf" (monthly stock file).
Type "/"
Sleep 300ms
Type "msf"
Sleep 1s
Enter
Sleep 2s

# ─── 6. VIEW TABLE METADATA ───
# Press right to drill into the table and see the preview pane.
Right
Sleep 3s

# ─── 7. SCROLL THROUGH COLUMNS ───
# Preview pane shows column catalog. Scroll with j/k.
Sleep 500ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 300ms
Down 1
Sleep 1s

# Scroll back up
Up 1
Sleep 200ms
Up 1
Sleep 200ms
Up 1
Sleep 1s

# ─── 8. FILTER COLUMNS ───
# Press "/" in the preview pane to filter columns.
Type "/"
Sleep 300ms
Type "ret"
Sleep 1s
# Press Escape to clear the filter
Escape
Sleep 1s

# ─── 9. OPEN DOWNLOAD DIALOG ───
# Press "d" to open the download form.
Type "d"
Sleep 1s

# ─── 10. FILL IN DOWNLOAD FORM ───
# SELECT columns field — clear prepopulated "*" then type columns
Ctrl+U
Sleep 200ms
Type "permno,date,prc,ret,shrout"
Sleep 500ms

# Move to WHERE clause
Tab
Sleep 300ms
Type "date >= '2024-01-01' AND date < '2024-02-01'"
Sleep 500ms

# Move to LIMIT — leave empty (no limit)
Tab
Sleep 300ms

# Move to Output path
Tab
Sleep 300ms
# Clear prepopulated path then type custom path
Ctrl+U
Sleep 200ms
Type "./crsp_msf_2024_jan.parquet"
Sleep 500ms

# Move to Format — keep default "parquet"
Tab
Sleep 1s

# ─── 11. SUBMIT DOWNLOAD ───
Enter
Sleep 5s

# ─── 12. DISMISS SUCCESS MESSAGE ───
Escape
Sleep 1s

# ─── 13. QUIT ───
Type "q"
Sleep 500ms

# ─── 14. VERIFY OUTPUT ───
Type "du -h crsp_msf_2024_jan.parquet"
Enter
Sleep 3s
