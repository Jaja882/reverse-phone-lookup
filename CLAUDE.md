# Claude Code Instructions

This is a Go CLI tool for reverse phone number lookup.

## Project Structure

```
.
├── main.go          # All application code (single file)
├── go.mod           # Go module definition
├── go.sum           # Dependency checksums
├── README.md        # User documentation
├── RESEARCH.md      # Background research on phone lookup APIs
└── .github/
    └── workflows/
        └── release.yml  # CI for building release binaries
```

## Building

```bash
go build -o lookup .
```

For smaller binary:
```bash
go build -ldflags="-s -w" -o lookup .
```

## Testing

Run the tool with test numbers:
```bash
./lookup +14155551234
./lookup -spam +18005551234
./lookup -o +14155551234
```

## Key Dependencies

- `github.com/nyaruka/phonenumbers` - Google's libphonenumber port for Go

## Architecture

The code is intentionally kept in a single `main.go` file for simplicity. Main components:

1. **Phone lookup** (`lookupPhone`) - Parses and validates numbers using libphonenumber
2. **Spam detection** (`calculateSpamScore`) - Heuristic scoring based on number type and patterns
3. **Online reputation** (`checkOnlineReputation`) - Scrapes nomorobo.com for US numbers
4. **OSINT dorks** (`generateDorks`) - Creates Google search queries for investigation

## Adding Features

When adding new features:
- Keep everything in `main.go` unless it grows significantly
- Add new flags in `main()` using the `flag` package
- Update README.md with new usage examples
- Update this file if architecture changes

## Release Process

Releases are automated via GitHub Actions. To create a release:
1. Tag a commit: `git tag v1.0.0`
2. Push the tag: `git push origin v1.0.0`
3. CI will build binaries for all platforms and create a GitHub release
