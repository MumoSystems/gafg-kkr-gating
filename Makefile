GO_FILES := rerun-jira-pr-checks.go create-change-work-item.go check-work-item-approval.go
OUTPUT_DIR := .github/actions
BINARIES := $(patsubst %.go, $(OUTPUT_DIR)/%, $(GO_FILES))

all: clean $(BINARIES)

# Delete the binaries, but not the directory
clean:
	rm -f $(BINARIES)

# Only create the directory if it doesn't exist
$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

# Build rule: depends on the directory and the .go file
$(OUTPUT_DIR)/%: %.go $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 go build -o $@ $<
