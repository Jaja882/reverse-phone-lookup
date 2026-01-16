# Reverse Phone Lookup

A fast CLI tool for reverse phone number lookup with spam detection and OSINT capabilities.

Built with Go using [nyaruka/phonenumbers](https://github.com/nyaruka/phonenumbers) (Google's libphonenumber port).

## Features

- **Phone validation** - Parse and validate international phone numbers
- **Number info** - Type (mobile/landline/VoIP), carrier, location, timezone
- **Spam detection** - Heuristic scoring + online reputation check (nomorobo.com)
- **OSINT dorks** - Generate Google search queries for deeper investigation
- **Cross-platform** - Single static binary for Linux, macOS, Windows

## Installation

### Download binary

Download the latest release from the [Releases page](../../releases).

### Install with Go

```bash
go install github.com/stevemurr/reverse-phone-lookup@latest
```

The binary will be installed to `$GOPATH/bin` (usually `~/go/bin`). Make sure it's in your PATH.

### Build from source

```bash
git clone https://github.com/stevemurr/reverse-phone-lookup.git
cd reverse-phone-lookup
go build -o lookup .
```

## Usage

```bash
# Basic lookup
./lookup +14155551234

# With spam detection
./lookup -spam +18005551234

# Show OSINT search queries
./lookup -o +14155551234

# JSON output
./lookup -j -spam +14155551234

# Number without country code (specify with -c)
./lookup -c US 4155551234
```

## Output Example

```
Phone:     +1 415-555-1234
E.164:     +14155551234
National:  (415) 555-1234
Valid:     Yes
Type:      Fixed Line or Mobile
Carrier:   Unknown
Location:  San Francisco, CA
Timezone:  America/Los_Angeles

----------------------------------------
SPAM ANALYSIS
----------------------------------------
Verdict:   LOW RISK (score: 30/100)
Reasons:
  - Unknown carrier (may be VoIP or spoofed)
  - Contains sequential digits (potentially fake)
```

## Flags

| Flag | Description |
|------|-------------|
| `-c CODE` | ISO country code (US, GB, DE, etc.) for numbers without + prefix |
| `-j` | Output as JSON |
| `-spam` | Run spam analysis (includes online lookup) |
| `-o` | Show OSINT Google dork queries |
| `-s` | Open searches in browser (use with -o) |

## Spam Scoring

The spam detector uses heuristics and online reputation data:

| Factor | Points |
|--------|--------|
| Premium rate number | +50 |
| Toll-free number | +30 |
| Toll-free prefix (800, 888, etc.) | +20 |
| VoIP number | +25 |
| Unknown carrier | +15 |
| Repeating digits (11111) | +20 |
| Sequential digits (1234) | +15 |
| Reported on nomorobo.com | +40 |

**Verdicts:**
- `HIGH RISK` (70+) - Very likely spam
- `SUSPICIOUS` (40-69) - Exercise caution
- `LOW RISK` (20-39) - Probably fine
- `LIKELY SAFE` (0-19) - Low spam indicators

## Limitations

- **Carrier data is prefix-based** - May be inaccurate for ported numbers
- **No owner/name lookup** - Requires commercial APIs (Twilio, etc.)
- **Online reputation** - Only available for US numbers via nomorobo.com

## License

MIT
