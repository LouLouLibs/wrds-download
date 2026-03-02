.PHONY: demo demo-record demo-redact

# Record the VHS demo and blur the username in one step.
demo: demo-record demo-redact

# Step 1: Record the raw GIF with VHS (outputs demo-wrds-download-raw.gif).
demo-record:
	vhs demo-wrds-download.tape

# Step 2: Blur the username on the login screen (raw -> clean).
# Override blur coordinates: BLUR_X=500 BLUR_Y=280 make demo-redact
demo-redact:
	./scripts/redact-demo.sh demo-wrds-download-raw.gif demo-wrds-download.gif
