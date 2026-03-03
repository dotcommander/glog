package services

import (
	"regexp"

	"github.com/dotcommander/glog/internal/domain/entities"
)

// severityOrder is the ordered list of severity levels from highest to lowest.
// Declared once at package level to avoid per-call allocation.
var severityOrder = []entities.LogLevel{
	entities.LogLevelFatal,
	entities.LogLevelError,
	entities.LogLevelWarn,
	entities.LogLevelInfo,
	entities.LogLevelDebug,
	entities.LogLevelTrace,
}

// PatternMatcher analyzes log messages to derive metadata.
type PatternMatcher struct {
	severityPatterns map[entities.LogLevel][]*regexp.Regexp
	categoryPatterns []categoryPattern
	sourcePatterns   []*regexp.Regexp
}

// categoryPattern matches log categories (database, http, auth, etc.)
type categoryPattern struct {
	Category string
	Regex    *regexp.Regexp
	Priority int // Higher priority wins
}

// NewPatternMatcher creates a new pattern matcher with built-in patterns.
func NewPatternMatcher() *PatternMatcher {
	pm := &PatternMatcher{
		severityPatterns: make(map[entities.LogLevel][]*regexp.Regexp),
		categoryPatterns: make([]categoryPattern, 0),
	}

	// Initialize severity patterns
	pm.initSeverityPatterns()
	pm.initCategoryPatterns()
	pm.initSourcePatterns()

	return pm
}

// initSeverityPatterns sets up severity detection patterns.
func (pm *PatternMatcher) initSeverityPatterns() {
	// Fatal patterns
	pm.severityPatterns[entities.LogLevelFatal] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(fatal|panic|unrecoverable)`),
	}

	// Error patterns
	pm.severityPatterns[entities.LogLevelError] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(error|exception|failed|failure|unable to|could not|did not work)`),
		regexp.MustCompile(`(?i)(timed.?out|connection refused|connection reset|connection lost)`),
		regexp.MustCompile(`(?i)(syntax error|runtime error|stack trace|traceback|segmentation fault|segfault)`),
		regexp.MustCompile(`(?i)(permission denied|access denied|unauthorized|forbidden|invalid credentials)`),
		regexp.MustCompile(`(?i)(database error|sql error|query failed|transaction failed)`),
		regexp.MustCompile(`(?i)(http.*[4-5]\d{2}|status.*[4-5]\d{2}|response.*[4-5]\d{2})`),
		regexp.MustCompile(`(?i)(internal server error|service unavailable|bad gateway|gateway timeout)`),
	}

	// Warn patterns
	pm.severityPatterns[entities.LogLevelWarn] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(warn|warning|caution|deprecated|obsolete|legacy)`),
		regexp.MustCompile(`(?i)(slow|slow query|high latency|bottleneck|inefficient)`),
		regexp.MustCompile(`(?i)(retry|retried|attempt.*failed|fallback|degraded|unstable)`),
		regexp.MustCompile(`(?i)(disk.*(full|low)|memory.*(low|high)|cpu.*(high|overload))`),
		regexp.MustCompile(`(?i)(queue.*full|backpressure|throttled|rate limited)`),
		regexp.MustCompile(`(?i)(configuration warning|misconfigured|deprecated.*api)`),
	}

	// Info patterns (default, so fewer explicit matches)
	pm.severityPatterns[entities.LogLevelInfo] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(starting|started|running|executing|processing|working|active)`),
		regexp.MustCompile(`(?i)(completed|finished|done|success|ok|ready)`),
		regexp.MustCompile(`(?i)(loading|loaded|initialized|configured|setup)`),
	}

	// Debug patterns
	pm.severityPatterns[entities.LogLevelDebug] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(debug|trace|diagnostic|verbose|detail)`),
	}

	// Trace patterns
	pm.severityPatterns[entities.LogLevelTrace] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(trace|enter|exit|function call|method call|call stack)`),
	}
}

// initCategoryPatterns sets up category detection patterns.
func (pm *PatternMatcher) initCategoryPatterns() {
	pm.categoryPatterns = []categoryPattern{
		// HTTP category (high priority)
		{Category: "http", Priority: 90, Regex: regexp.MustCompile(`(?i)(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+/(\S+)`)},
		{Category: "http", Priority: 90, Regex: regexp.MustCompile(`(?i)(http|https)://\S+`)},
		{Category: "http", Priority: 85, Regex: regexp.MustCompile(`(?i)(status code|response status|http status).*[1-5]\d{2}`)},
		{Category: "http", Priority: 80, Regex: regexp.MustCompile(`(?i)(request|response|endpoint|route|api|url|uri)`)},

		// Database category
		{Category: "database", Priority: 90, Regex: regexp.MustCompile(`(?i)(mysql|postgres|postgresql|sqlite|mongodb|redis|cassandra|elasticsearch)`)},
		{Category: "database", Priority: 85, Regex: regexp.MustCompile(`(?i)(sql|query|select|insert|update|delete|create|alter|drop|index|transaction|commit|rollback)`)},
		{Category: "database", Priority: 80, Regex: regexp.MustCompile(`(?i)(connection|pool|timeout|lock|deadlock|migration|schema)`)},

		// Authentication/Security category
		{Category: "auth", Priority: 90, Regex: regexp.MustCompile(`(?i)(login|logout|signin|signout|authenticate|authentication|authorize|authorization|token|jwt|oauth|saml|ldap)`)},
		{Category: "auth", Priority: 85, Regex: regexp.MustCompile(`(?i)(user|account|role|permission|access|privilege|session|cookie)`)},
		{Category: "auth", Priority: 80, Regex: regexp.MustCompile(`(?i)(password|credential|secret|key|certificate|ssl|tls|encryption|hash|bcrypt|argon2)`)},

		// Error/Exception category
		{Category: "error", Priority: 95, Regex: regexp.MustCompile(`(?i)(exception|stack trace|traceback|segmentation fault|segfault|panic|abort|core dump)`)},
		{Category: "error", Priority: 90, Regex: regexp.MustCompile(`(?i)(error|failed|failure|unable|could not|did not|invalid|illegal|bad|wrong)`)},

		// Performance category
		{Category: "performance", Priority: 85, Regex: regexp.MustCompile(`(?i)(slow|latency|response time|throughput|rps|qps|load|cpu|memory|disk|io|network|bandwidth)`)},
		{Category: "performance", Priority: 80, Regex: regexp.MustCompile(`(?i)(optimized|optimization|cache|caching|index|indexed|query plan|explain)`)},

		// Application/Service category (lower priority - catch-all)
		{Category: "service", Priority: 70, Regex: regexp.MustCompile(`(?i)(service|microservice|daemon|worker|job|task|queue|pubsub|event|message|handler)`)},
		{Category: "service", Priority: 60, Regex: regexp.MustCompile(`(?i)(start|stop|restart|deploy|deployment|upgrade|version|config|configuration|environment)`)},

		// Security category
		{Category: "security", Priority: 80, Regex: regexp.MustCompile(`(?i)(attack|breach|intrusion|malware|virus|trojan|ransomware|xss|sql injection|csrf)`)},
		{Category: "security", Priority: 75, Regex: regexp.MustCompile(`(?i)(firewall|ips|ids|vpn|proxy|certificate|ca|trust|verify|validation)`)},

		// Network category
		{Category: "network", Priority: 85, Regex: regexp.MustCompile(`(?i)(tcp|udp|ip|dns|dhcp|ping|traceroute|route|routing|switch|router|gateway|load balancer)`)},
		{Category: "network", Priority: 80, Regex: regexp.MustCompile(`(?i)(connection|socket|port|packet|traffic|bandwidth|latency|jitter|drop|loss)`)},
	}
}

// initSourcePatterns pre-compiles the regexes used by deriveSource.
func (pm *PatternMatcher) initSourcePatterns() {
	pm.sourcePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\[([^\]]+)\]`),
		regexp.MustCompile(`(?i)^([a-zA-Z0-9_-]+)\s*[:|]\s*`),
		regexp.MustCompile(`(?i)(?:from|in)\s+([a-zA-Z0-9_-]+)`),
	}
}

// Analyze takes a log and derives metadata from its message and fields.
func (pm *PatternMatcher) Analyze(log *entities.Log) {
	message := log.Message

	// Derive severity if not explicitly set or if it seems too low
	if derivedLevel := pm.deriveSeverity(message, log.Level); derivedLevel != "" && string(log.Level) != string(derivedLevel) {
		levelStr := string(derivedLevel)
		log.DerivedLevel = &levelStr
	}

	// Derive source if not already set
	if log.DerivedSource == nil || *log.DerivedSource == "" {
		if source := pm.deriveSource(message); source != "" {
			log.DerivedSource = &source
		}
	}

	// Derive category
	if log.DerivedCategory == nil || *log.DerivedCategory == "" {
		if category := pm.deriveCategory(message, log.Fields); category != "" {
			log.DerivedCategory = &category
		}
	}
}

// deriveSeverity analyzes the message to determine the appropriate log level.
func (pm *PatternMatcher) deriveSeverity(message string, currentLevel entities.LogLevel) entities.LogLevel {
	// If already error/fatal, don't downgrade
	if currentLevel == entities.LogLevelError || currentLevel == entities.LogLevelFatal {
		return ""
	}

	// Check each severity level in order (fatal > error > warn > info > debug > trace)
	for _, level := range severityOrder {
		patterns, exists := pm.severityPatterns[level]
		if !exists {
			continue
		}

		for _, pattern := range patterns {
			if pattern.MatchString(message) {
				// If we found a higher severity than current, use it
				if pm.isHigherSeverity(level, currentLevel) {
					return level
				}
				return ""
			}
		}
	}

	return ""
}

// isHigherSeverity returns true if level1 is higher (more severe) than level2.
func (pm *PatternMatcher) isHigherSeverity(level1, level2 entities.LogLevel) bool {
	return entities.LogPriority[level1] > entities.LogPriority[level2]
}

// deriveSource extracts the service/component name from the log message.
func (pm *PatternMatcher) deriveSource(message string) string {
	// Pattern for [service-name] format
	if matches := pm.sourcePatterns[0].FindStringSubmatch(message); len(matches) > 1 {
		return matches[1]
	}

	// Pattern for "service: message" format
	if matches := pm.sourcePatterns[1].FindStringSubmatch(message); len(matches) > 1 {
		return matches[1]
	}

	// Pattern for "from service" or "in service"
	if matches := pm.sourcePatterns[2].FindStringSubmatch(message); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// deriveCategory determines the log category based on message content.
func (pm *PatternMatcher) deriveCategory(message string, fields entities.JSONMap) string {
	// Find best matching category (all regexes use (?i) flag — no ToLower needed)
	bestMatch := ""
	highestPriority := -1

	for _, pattern := range pm.categoryPatterns {
		if pattern.Regex.MatchString(message) {
			if pattern.Priority > highestPriority {
				highestPriority = pattern.Priority
				bestMatch = pattern.Category
			}
		}
	}

	// If no category matched, try to derive from fields
	if bestMatch == "" && fields != nil {
		// Check for HTTP-specific fields
		if _, hasMethod := fields["http_method"]; hasMethod {
			return "http"
		}
		if _, hasStatus := fields["http_status"]; hasStatus {
			return "http"
		}

		// Check for error fields
		if _, hasError := fields["error"]; hasError {
			return "error"
		}
		if _, hasException := fields["exception"]; hasException {
			return "error"
		}

		// Check for performance fields
		if _, hasDuration := fields["duration_ms"]; hasDuration {
			return "performance"
		}
	}

	return bestMatch
}
