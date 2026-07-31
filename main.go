package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nyaruka/phonenumbers"
)

// Toll-free prefixes commonly used in scams
var tollFreePrefixes = map[string]bool{
	"800": true, "888": true, "877": true, "866": true,
	"855": true, "844": true, "833": true,
}

// PhoneResult holds all lookup information
type PhoneResult struct {
	Input         string    `json:"input"`
	E164          string    `json:"e164"`
	International string    `json:"international"`
	National      string    `json:"national"`
	CountryCode   int32     `json:"country_code"`
	NationalNum   uint64    `json:"national_number"`
	Valid         bool      `json:"valid"`
	Possible      bool      `json:"possible"`
	Type          string    `json:"type"`
	Carrier       string    `json:"carrier,omitempty"`
	Location      string    `json:"location,omitempty"`
	Timezones     []string  `json:"timezones,omitempty"`
	Spam          *SpamInfo `json:"spam,omitempty"`
}

// SpamInfo holds spam analysis results
type SpamInfo struct {
	Score        int      `json:"score"`
	Verdict      string   `json:"verdict"`
	Reasons      []string `json:"reasons"`
	ReportedSpam *bool    `json:"reported_spam,omitempty"`
	Source       string   `json:"source,omitempty"`
}

// Dork represents a Google search query
type Dork struct {
	Description string `json:"description"`
	Query       string `json:"query"`
	Category    string `json:"category"`
}

func getNumberTypeName(numType phonenumbers.PhoneNumberType) string {
	types := map[phonenumbers.PhoneNumberType]string{
		phonenumbers.FIXED_LINE:           "Fixed Line",
		phonenumbers.MOBILE:               "Mobile",
		phonenumbers.FIXED_LINE_OR_MOBILE: "Fixed Line or Mobile",
		phonenumbers.TOLL_FREE:            "Toll Free",
		phonenumbers.PREMIUM_RATE:         "Premium Rate",
		phonenumbers.SHARED_COST:          "Shared Cost",
		phonenumbers.VOIP:                 "VoIP",
		phonenumbers.PERSONAL_NUMBER:      "Personal Number",
		phonenumbers.PAGER:                "Pager",
		phonenumbers.UAN:                  "UAN",
		phonenumbers.VOICEMAIL:            "Voicemail",
		phonenumbers.UNKNOWN:              "Unknown",
	}
	if name, ok := types[numType]; ok {
		return name
	}
	return "Unknown"
}

func lookupPhone(phoneStr, countryCode string, checkSpam bool) (*PhoneResult, *phonenumbers.PhoneNumber, error) {
	var parsed *phonenumbers.PhoneNumber
	var err error

	if countryCode != "" {
		parsed, err = phonenumbers.Parse(phoneStr, countryCode)
	} else {
		parsed, err = phonenumbers.Parse(phoneStr, "")
	}
	if err != nil {
		return nil, nil, err
	}

	isValid := phonenumbers.IsValidNumber(parsed)
	isPossible := phonenumbers.IsPossibleNumber(parsed)

	result := &PhoneResult{
		Input:         phoneStr,
		E164:          phonenumbers.Format(parsed, phonenumbers.E164),
		International: phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL),
		National:      phonenumbers.Format(parsed, phonenumbers.NATIONAL),
		CountryCode:   parsed.GetCountryCode(),
		NationalNum:   parsed.GetNationalNumber(),
		Valid:         isValid,
		Possible:      isPossible,
	}

	if isValid {
		numType := phonenumbers.GetNumberType(parsed)
		result.Type = getNumberTypeName(numType)

		carrierName, _ := phonenumbers.GetCarrierForNumber(parsed, "en")
		if carrierName == "" {
			carrierName = "Unknown"
		}
		result.Carrier = carrierName

		location, _ := phonenumbers.GetGeocodingForNumber(parsed, "en")
		if location == "" {
			location = "Unknown"
		}
		result.Location = location

		result.Timezones, _ = phonenumbers.GetTimezonesForNumber(parsed)

		if checkSpam {
			result.Spam = calculateSpamScore(parsed, numType, carrierName)
			checkOnlineReputation(parsed, result.Spam)
		}
	} else {
		result.Type = "Invalid"
	}

	return result, parsed, nil
}

func calculateSpamScore(parsed *phonenumbers.PhoneNumber, numType phonenumbers.PhoneNumberType, carrierName string) *SpamInfo {
	score := 0
	var reasons []string

	national := fmt.Sprintf("%d", parsed.GetNationalNumber())
	areaCode := ""
	if len(national) >= 3 {
		areaCode = national[:3]
	}

	// Number type scoring
	switch numType {
	case phonenumbers.TOLL_FREE:
		score += 30
		reasons = append(reasons, "Toll-free numbers are commonly used for spam/robocalls")
	case phonenumbers.PREMIUM_RATE:
		score += 50
		reasons = append(reasons, "Premium rate numbers are high-risk for scams")
	case phonenumbers.VOIP:
		score += 25
		reasons = append(reasons, "VoIP numbers are easier to obtain anonymously")
	case phonenumbers.PERSONAL_NUMBER:
		score += 15
		reasons = append(reasons, "Personal numbers can be used for spoofing")
	}

	// Toll-free prefix check
	if tollFreePrefixes[areaCode] {
		score += 20
		reasons = append(reasons, fmt.Sprintf("Toll-free prefix (%s) commonly used in scams", areaCode))
	}

	// Unknown carrier is suspicious
	if carrierName == "Unknown" || carrierName == "" {
		score += 15
		reasons = append(reasons, "Unknown carrier (may be VoIP or spoofed)")
	}

	// Repeating digit patterns (check for 5+ of the same digit)
	for _, digit := range "0123456789" {
		repeated := strings.Repeat(string(digit), 5)
		if strings.Contains(national, repeated) {
			score += 20
			reasons = append(reasons, "Contains suspicious repeating digits")
			break
		}
	}

	// Sequential digits
	sequential := regexp.MustCompile(`(0123|1234|2345|3456|4567|5678|6789|7890)`)
	if sequential.MatchString(national) {
		score += 15
		reasons = append(reasons, "Contains sequential digits (potentially fake)")
	}

	if score > 100 {
		score = 100
	}

	verdict := getSpamVerdict(score)

	return &SpamInfo{
		Score:   score,
		Verdict: verdict,
		Reasons: reasons,
	}
}

func getSpamVerdict(score int) string {
	switch {
	case score >= 70:
		return "HIGH RISK"
	case score >= 40:
		return "SUSPICIOUS"
	case score >= 20:
		return "LOW RISK"
	default:
		return "LIKELY SAFE"
	}
}

func checkOnlineReputation(parsed *phonenumbers.PhoneNumber, spam *SpamInfo) {
	national := fmt.Sprintf("%d", parsed.GetNationalNumber())

	// Only check US numbers with 10 digits
	if parsed.GetCountryCode() != 1 || len(national) != 10 {
		return
	}

	url := fmt.Sprintf("https://www.nomorobo.com/lookup/%s-%s-%s",
		national[:3], national[3:6], national[6:])

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	html := string(body)

	if strings.Contains(html, "Do Not Answer") ||
		strings.Contains(html, "Robocaller") ||
		strings.Contains(html, "Political") {
		reported := true
		spam.ReportedSpam = &reported
		spam.Source = "nomorobo.com"
		spam.Score = min(spam.Score+40, 100)
		spam.Verdict = getSpamVerdict(spam.Score)
		spam.Reasons = append(spam.Reasons, "Reported as spam/robocall on nomorobo.com")
	} else if strings.Contains(html, "Safe") {
		reported := false
		spam.ReportedSpam = &reported
		spam.Source = "nomorobo.com"
	}
}

func generatePhoneFormats(parsed *phonenumbers.PhoneNumber) []string {
	formats := make(map[string]bool)

	e164 := phonenumbers.Format(parsed, phonenumbers.E164)
	intl := phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
	national := phonenumbers.Format(parsed, phonenumbers.NATIONAL)

	formats[e164] = true
	formats[intl] = true
	formats[national] = true

	raw := fmt.Sprintf("%d%d", parsed.GetCountryCode(), parsed.GetNationalNumber())
	nationalDigits := fmt.Sprintf("%d", parsed.GetNationalNumber())

	formats[raw] = true
	formats[nationalDigits] = true

	// US format variations
	if len(nationalDigits) == 10 {
		formats[fmt.Sprintf("(%s) %s-%s", nationalDigits[:3], nationalDigits[3:6], nationalDigits[6:])] = true
		formats[fmt.Sprintf("%s-%s-%s", nationalDigits[:3], nationalDigits[3:6], nationalDigits[6:])] = true
		formats[fmt.Sprintf("%s.%s.%s", nationalDigits[:3], nationalDigits[3:6], nationalDigits[6:])] = true
	}

	result := make([]string, 0, len(formats))
	for f := range formats {
		result = append(result, f)
	}
	return result
}

func generateDorks(parsed *phonenumbers.PhoneNumber) []Dork {
	formats := generatePhoneFormats(parsed)
	e164 := phonenumbers.Format(parsed, phonenumbers.E164)
	national := fmt.Sprintf("%d", parsed.GetNationalNumber())

	var dorks []Dork

	// General searches (limit to 4)
	for i, f := range formats {
		if i >= 4 {
			break
		}
		dorks = append(dorks, Dork{
			Description: fmt.Sprintf("General search for %s", f),
			Query:       fmt.Sprintf(`"%s"`, f),
			Category:    "general",
		})
	}

	// Social media
	socialSites := []struct {
		name  string
		query string
	}{
		{"Facebook", "site:facebook.com"},
		{"LinkedIn", "site:linkedin.com"},
		{"Twitter/X", "site:twitter.com OR site:x.com"},
		{"Instagram", "site:instagram.com"},
	}

	for _, site := range socialSites {
		dorks = append(dorks, Dork{
			Description: fmt.Sprintf("%s profiles", site.name),
			Query:       fmt.Sprintf(`%s "%s" OR "%s"`, site.query, e164, national),
			Category:    "social",
		})
	}

	// People search sites
	dorks = append(dorks, Dork{
		Description: "People search sites",
		Query:       fmt.Sprintf(`(site:whitepages.com OR site:truepeoplesearch.com OR site:fastpeoplesearch.com OR site:spokeo.com) "%s"`, national),
		Category:    "directory",
	})

	// Scam/spam reports
	dorks = append(dorks, Dork{
		Description: "Scam/spam reports",
		Query:       fmt.Sprintf(`(site:800notes.com OR site:whocallsme.com OR site:callercomplaints.com OR "spam" OR "scam") "%s"`, national),
		Category:    "reputation",
	})

	// Business listings
	dorks = append(dorks, Dork{
		Description: "Business listings",
		Query:       fmt.Sprintf(`(site:yelp.com OR site:yellowpages.com OR site:bbb.org) "%s"`, national),
		Category:    "business",
	})

	// Documents
	dorks = append(dorks, Dork{
		Description: "Documents (PDF, DOC)",
		Query:       fmt.Sprintf(`(filetype:pdf OR filetype:doc OR filetype:docx) "%s"`, national),
		Category:    "documents",
	})

	return dorks
}

func printResult(result *PhoneResult, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	fmt.Printf("Phone:     %s\n", result.International)
	fmt.Printf("E.164:     %s\n", result.E164)
	fmt.Printf("National:  %s\n", result.National)
	fmt.Printf("Valid:     %s\n", boolToYesNo(result.Valid))

	if result.Valid {
		fmt.Printf("Type:      %s\n", result.Type)
		fmt.Printf("Carrier:   %s\n", result.Carrier)
		fmt.Printf("Location:  %s\n", result.Location)
		if len(result.Timezones) > 0 {
			fmt.Printf("Timezone:  %s\n", strings.Join(result.Timezones, ", "))
		}

		if result.Spam != nil {
			fmt.Println()
			fmt.Println("----------------------------------------")
			fmt.Println("SPAM ANALYSIS")
			fmt.Println("----------------------------------------")
			fmt.Printf("Verdict:   %s (score: %d/100)\n", result.Spam.Verdict, result.Spam.Score)

			if result.Spam.ReportedSpam != nil {
				if *result.Spam.ReportedSpam {
					fmt.Printf("Online:    REPORTED AS SPAM (%s)\n", result.Spam.Source)
				} else {
					fmt.Printf("Online:    Not reported (%s)\n", result.Spam.Source)
				}
			}

			if len(result.Spam.Reasons) > 0 {
				fmt.Println("Reasons:")
				for _, reason := range result.Spam.Reasons {
					fmt.Printf("  - %s\n", reason)
				}
			}
		}
	}
}

func printDorks(parsed *phonenumbers.PhoneNumber, openInBrowser bool, showURLs bool) {
	dorks := generateDorks(parsed)

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("OSINT Search Queries")
	fmt.Println("==================================================")

	categories := make(map[string][]Dork)
	for _, dork := range dorks {
		categories[dork.Category] = append(categories[dork.Category], dork)
	}

	var urls []string
	for cat, catDorks := range categories {
		fmt.Printf("\n[%s]\n", strings.ToUpper(cat))
		for _, dork := range catDorks {
			fmt.Printf("  %s:\n", dork.Description)
			fmt.Printf("    %s\n", dork.Query)
			if showURLs {
				fmt.Printf("    URL: %s\n", buildSearchURL(dork.Query))
			}
			if openInBrowser {
				urls = append(urls, buildSearchURL(dork.Query))
			}
		}
	}

	if openInBrowser && len(urls) > 0 {
		fmt.Printf("\nOpening searches in browser...\n")
		for _, url := range urls {
			if err := openBrowser(url); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open browser for %s: %v\n", url, err)
			}
		}
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("python3"); err == nil {
			cmd = exec.Command("python3", "-m", "webbrowser", url)
		} else {
			return fmt.Errorf("no browser opener available: install xdg-open or python3")
		}
	}
	return cmd.Start()
}

func buildSearchURL(query string) string {
	return fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(query))
}

func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func main() {
	countryCode := flag.String("c", "", "ISO country code (e.g., US, GB, DE) for numbers without + prefix")
	asJSON := flag.Bool("j", false, "Output result as JSON")
	osint := flag.Bool("o", false, "Show OSINT search queries (Google dorks)")
	urls := flag.Bool("u", false, "Print OSINT Google search URLs (use with -o)")
	search := flag.Bool("s", false, "Open OSINT searches in browser (use with -o)")
	spam := flag.Bool("spam", false, "Check if number is likely spam/scam")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <phone>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Reverse phone number lookup tool with OSINT capabilities\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample: %s +14155551234 -spam\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	phone := flag.Arg(0)

	result, parsed, err := lookupPhone(phone, *countryCode, *spam)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	printResult(result, *asJSON)

	if !result.Valid {
		os.Exit(1)
	}

	if *osint && !*asJSON && parsed != nil {
		printDorks(parsed, *search, *urls)
	}
}
