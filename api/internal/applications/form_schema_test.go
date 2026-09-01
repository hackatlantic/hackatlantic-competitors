package applications

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const hackAtlanticFormV2 = `{
  "resumeRequired": false,
  "resumeAfterQuestionKey": "school",
  "questions": [
    {
      "key": "fullName",
      "label": "Name",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "email",
      "label": "Email",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "email",
      "help": "Verified through your signed-in account."
    },
    {
      "key": "school",
      "label": "School",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "dietaryRestrictions",
      "label": "Dietary restrictions",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text",
      "help": "Enter None if you do not have any."
    },
    {
      "key": "hackAtlanticExcitement",
      "label": "What are you most excited about at Hack Atlantic?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "maxWords": 100,
      "help": "Maximum 100 words."
    },
    {
      "key": "priorHackathonExperience",
      "label": "Prior hackathon experience",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "select",
      "options": ["This is my first", "1–3", "3+"]
    },
    {
      "key": "desiredTeammateNames",
      "label": "Desired teammate names (Optional)",
      "type": "string",
      "required": false,
      "section": "Hackathon Specific Questions",
      "control": "text"
    },
    {
      "key": "hardwareProject",
      "label": "Are you looking to make a hardware project?",
      "type": "boolean",
      "required": true,
      "section": "Hackathon Specific Questions"
    },
    {
      "key": "hardwareEquipment",
      "label": "What equipment are you looking to use?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "showWhen": {
        "key": "hardwareProject",
        "equals": true
      }
    }
  ]
}`

func encodedAnswers(values map[string]any) map[string]json.RawMessage {
	answers := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		encoded, _ := json.Marshal(value)
		answers[key] = encoded
	}
	return answers
}

func completeHackAtlanticAnswers() map[string]json.RawMessage {
	return encodedAnswers(map[string]any{
		"fullName":                 "Ada Lovelace",
		"email":                    "ada@example.com",
		"school":                   "Atlantic University",
		"dietaryRestrictions":      "None",
		"hackAtlanticExcitement":   "Building useful things with thoughtful people.",
		"priorHackathonExperience": "This is my first",
		"hardwareProject":          true,
		"hardwareEquipment":        "Arduino and a soldering station",
	})
}

func TestHackAtlanticFormV2ParsesAndValidates(t *testing.T) {
	form, err := parsePublishedForm([]byte(hackAtlanticFormV2))
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	if form.ResumeRequired {
		t.Fatal("resume should be optional")
	}
	if form.ResumeAfterQuestionKey == nil || *form.ResumeAfterQuestionKey != "school" {
		t.Fatalf("unexpected resume position: %v", form.ResumeAfterQuestionKey)
	}
	if err := validateAnswers(form, completeHackAtlanticAnswers(), true); err != nil {
		t.Fatalf("validate complete answers: %v", err)
	}
}

func TestHackAtlanticFormV2EnforcesConditionalEquipment(t *testing.T) {
	form, _ := parsePublishedForm([]byte(hackAtlanticFormV2))
	answers := completeHackAtlanticAnswers()
	delete(answers, "hardwareEquipment")
	if err := validateAnswers(form, answers, true); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("missing equipment error = %v, want incomplete", err)
	}

	answers = completeHackAtlanticAnswers()
	answers["hardwareProject"] = json.RawMessage("false")
	delete(answers, "hardwareEquipment")
	if err := validateAnswers(form, answers, true); err != nil {
		t.Fatalf("hardware=no should not require equipment: %v", err)
	}
}

func TestHackAtlanticFormV2RejectsInvalidChoiceEmailAndWordLimit(t *testing.T) {
	form, _ := parsePublishedForm([]byte(hackAtlanticFormV2))
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "choice", key: "priorHackathonExperience", value: "Several"},
		{name: "email", key: "email", value: "not-an-email"},
		{name: "word limit", key: "hackAtlanticExcitement", value: strings.Repeat("word ", 101)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answers := completeHackAtlanticAnswers()
			encoded, _ := json.Marshal(test.value)
			answers[test.key] = encoded
			if err := validateAnswers(form, answers, true); !errors.Is(err, ErrInvalidAnswers) {
				t.Fatalf("validation error = %v, want invalid answers", err)
			}
		})
	}
}
