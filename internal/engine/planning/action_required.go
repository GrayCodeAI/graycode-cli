package planning

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ActionRequired represents a structured mid-task data collection request
// presented to the user during agent execution.
type ActionRequired struct {
	ID          string
	Title       string
	Description string
	Fields      []FormField
	Timeout     time.Duration
	Required    bool
	CreatedAt   time.Time
	Response    *FormResponse
	Resolved    bool
}

// FormField defines a single input field in an action-required form.
type FormField struct {
	Name       string
	Label      string
	Type       string // "text", "choice", "boolean", "number", "password"
	Required   bool
	Default    string
	Choices    []string
	Validation string // regex pattern
}

// FormResponse holds the user's submitted values for an action-required form.
type FormResponse struct {
	Values      map[string]string
	SubmittedAt time.Time
	TimedOut    bool
}

// ActionManager coordinates action-required requests, managing pending and
// historical forms and delegating presentation to the configured PromptFn.
type ActionManager struct {
	Pending  []*ActionRequired
	History  []*ActionRequired
	PromptFn func(action *ActionRequired) (*FormResponse, error)
	mu       sync.Mutex
}

// NewActionManager creates an ActionManager with the given prompt function.
func NewActionManager(promptFn func(*ActionRequired) (*FormResponse, error)) *ActionManager {
	return &ActionManager{
		Pending:  make([]*ActionRequired, 0),
		History:  make([]*ActionRequired, 0),
		PromptFn: promptFn,
	}
}

// Request presents an action-required form to the user via PromptFn, waits for
// a response or timeout, validates the response, and returns it.
func (am *ActionManager) Request(action *ActionRequired) (*FormResponse, error) {
	if action.CreatedAt.IsZero() {
		action.CreatedAt = time.Now()
	}

	am.mu.Lock()
	am.Pending = append(am.Pending, action)
	am.mu.Unlock()

	var resp *FormResponse
	var err error

	if action.Timeout > 0 {
		type result struct {
			resp *FormResponse
			err  error
		}
		ch := make(chan result, 1)
		go func() {
			r, e := am.PromptFn(action)
			ch <- result{r, e}
		}()
		select {
		case res := <-ch:
			resp = res.resp
			err = res.err
		case <-time.After(action.Timeout):
			resp = &FormResponse{
				Values:      make(map[string]string),
				SubmittedAt: time.Now(),
				TimedOut:    true,
			}
		}
	} else {
		resp, err = am.PromptFn(action)
	}

	am.mu.Lock()
	action.Response = resp
	action.Resolved = true
	// Move from Pending to History.
	newPending := make([]*ActionRequired, 0, len(am.Pending))
	for _, a := range am.Pending {
		if a.ID != action.ID {
			newPending = append(newPending, a)
		}
	}
	am.Pending = newPending
	am.History = append(am.History, action)
	am.mu.Unlock()

	if err != nil {
		return nil, err
	}

	// Validate if we got a real response (not timed out).
	if resp != nil && !resp.TimedOut {
		errs := Validate(action, resp)
		if len(errs) > 0 {
			return resp, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
		}
	}

	return resp, nil
}

// RequestText is a convenience method that requests a single text input from
// the user.
func (am *ActionManager) RequestText(title, description string) (string, error) {
	action := &ActionRequired{
		ID:          fmt.Sprintf("text-%d", time.Now().UnixNano()),
		Title:       title,
		Description: description,
		Fields: []FormField{
			{
				Name:     "value",
				Label:    title,
				Type:     "text",
				Required: true,
			},
		},
		Required: true,
	}
	resp, err := am.Request(action)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("no response received")
	}
	return resp.Values["value"], nil
}

// RequestChoice is a convenience method that requests a single choice from the
// user.
func (am *ActionManager) RequestChoice(title string, choices []string) (string, error) {
	action := &ActionRequired{
		ID:          fmt.Sprintf("choice-%d", time.Now().UnixNano()),
		Title:       title,
		Description: "Please select one option:",
		Fields: []FormField{
			{
				Name:     "value",
				Label:    title,
				Type:     "choice",
				Required: true,
				Choices:  choices,
			},
		},
		Required: true,
	}
	resp, err := am.Request(action)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("no response received")
	}
	return resp.Values["value"], nil
}

// RequestConfirm is a convenience method that requests a yes/no confirmation
// from the user.
func (am *ActionManager) RequestConfirm(title string) (bool, error) {
	action := &ActionRequired{
		ID:          fmt.Sprintf("confirm-%d", time.Now().UnixNano()),
		Title:       title,
		Description: "Please confirm:",
		Fields: []FormField{
			{
				Name:     "value",
				Label:    title,
				Type:     "boolean",
				Required: true,
			},
		},
		Required: true,
	}
	resp, err := am.Request(action)
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, fmt.Errorf("no response received")
	}
	val := strings.ToLower(resp.Values["value"])
	return val == "true" || val == "yes" || val == "y", nil
}

// Validate checks a FormResponse against the ActionRequired field specs.
// It returns a slice of validation error strings (empty if valid).
func Validate(action *ActionRequired, response *FormResponse) []string {
	var errs []string
	for _, field := range action.Fields {
		val, exists := response.Values[field.Name]
		if field.Required && (!exists || val == "") {
			errs = append(errs, fmt.Sprintf("field %q is required", field.Name))
			continue
		}
		if val == "" {
			continue
		}
		// Validate choice fields.
		if field.Type == "choice" && len(field.Choices) > 0 {
			found := false
			for _, c := range field.Choices {
				if c == val {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("field %q: invalid choice %q", field.Name, val))
			}
		}
		// Validate regex pattern.
		if field.Validation != "" {
			re, err := regexp.Compile(field.Validation)
			if err != nil {
				errs = append(errs, fmt.Sprintf("field %q: invalid validation pattern: %v", field.Name, err))
			} else if !re.MatchString(val) {
				errs = append(errs, fmt.Sprintf("field %q: value %q does not match pattern %q", field.Name, val, field.Validation))
			}
		}
	}
	return errs
}

// BuildFormPrompt renders a human-readable form prompt for an ActionRequired.
func BuildFormPrompt(action *ActionRequired) string {
	var b strings.Builder
	b.WriteString("── Action Required ─────────────────────────\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", action.Title))
	if action.Description != "" {
		b.WriteString(fmt.Sprintf("\n%s\n", action.Description))
	}
	b.WriteString("\nPlease provide:\n")
	for i, field := range action.Fields {
		meta := field.Type
		if field.Required {
			meta += ", required"
		}
		if field.Default != "" {
			meta += fmt.Sprintf(", default: %s", field.Default)
		}
		line := fmt.Sprintf("%d. %s [%s]:", i+1, field.Label, meta)
		if field.Type == "choice" && len(field.Choices) > 0 {
			line += " " + strings.Join(field.Choices, " / ")
		} else {
			line += " ___"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("─────────────────────────────────────────\n")
	return b.String()
}

// FormatResponse renders a FormResponse as a human-readable string.
func FormatResponse(response *FormResponse) string {
	if response == nil {
		return "<no response>"
	}
	if response.TimedOut {
		return "<timed out>"
	}
	var b strings.Builder
	b.WriteString("Response submitted at " + response.SubmittedAt.Format(time.RFC3339) + ":\n")
	for k, v := range response.Values {
		b.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	return b.String()
}

// GetPending returns the currently pending action-required requests.
func (am *ActionManager) GetPending() []*ActionRequired {
	am.mu.Lock()
	defer am.mu.Unlock()
	out := make([]*ActionRequired, len(am.Pending))
	copy(out, am.Pending)
	return out
}

// Cancel removes a pending action-required by ID and marks it resolved.
func (am *ActionManager) Cancel(id string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	newPending := make([]*ActionRequired, 0, len(am.Pending))
	for _, a := range am.Pending {
		if a.ID == id {
			a.Resolved = true
			am.History = append(am.History, a)
		} else {
			newPending = append(newPending, a)
		}
	}
	am.Pending = newPending
}
