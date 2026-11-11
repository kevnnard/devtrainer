package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

// lessonYAML is goYAML with a lesson on go.vars.
const lessonYAML = `
track:
  id: go
  title: Go
  order: 1
concepts:
  - id: go.vars
    title: Variables
    level: beginner
    lesson:
      intro: Go declares a variable and its type together.
      example:
        lang: go
        code: |
          var n int
          s := "hi"
          fmt.Println(n, s)
      steps:
        - text: var n int declares n with the zero value 0.
        - text: 'The := operator declares and assigns in one go.'
          predict:
            prompt: What type does s get?
            options:
              - {text: string, correct: true}
              - {text: any, explanation: 'The := operator infers the type of the value.'}
      takeaway: Every variable has a type and starts at its zero value.
    items:
      - id: go.vars.1
        type: mcq
        prompt: Which declares and assigns?
        options:
          - {text: "x := 1", correct: true}
          - {text: "x = 1", misconception: assign-declares, explanation: = only assigns an existing variable.}
`

func TestLoadLesson(t *testing.T) {
	cat, err := Load(fstest.MapFS{"go/go.yaml": {Data: []byte(lessonYAML)}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c, _ := cat.Concept("go.vars")
	l := c.Lesson
	if l == nil {
		t.Fatal("go.vars has no lesson")
	}
	if l.Intro == "" || l.Takeaway == "" || l.Example.Lang != "go" || !strings.HasPrefix(l.Example.Code, "var n int") {
		t.Errorf("lesson = %+v", l)
	}
	if len(l.Steps) != 2 || l.Steps[0].Predict != nil || l.Steps[1].Predict == nil {
		t.Fatalf("steps = %+v", l.Steps)
	}
	p := l.Steps[1].Predict
	if p.Prompt != "What type does s get?" || len(p.Options) != 2 || p.Answer() != 0 || p.Options[1].Explanation == "" {
		t.Errorf("predict = %+v", p)
	}

	// A concept without a lesson keeps a nil Lesson.
	cat, err = Load(fstest.MapFS{"go/go.yaml": {Data: []byte(goYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := cat.Concept("go.vars"); c.Lesson != nil {
		t.Errorf("lesson = %+v, want nil", c.Lesson)
	}
}

func TestLoadLessonErrors(t *testing.T) {
	longCode := "        code: |\n" + strings.Repeat("          x++\n", 16)
	cases := []struct {
		name     string
		old, new string
		parts    []string
	}{
		{"no steps",
			"      steps:\n        - text: var n int declares n with the zero value 0.\n        - text: 'The := operator declares and assigns in one go.'\n          predict:\n            prompt: What type does s get?\n            options:\n              - {text: string, correct: true}\n              - {text: any, explanation: 'The := operator infers the type of the value.'}\n",
			"      steps: []\n",
			[]string{"go/go.yaml", "go.vars", "lesson", "no steps"}},
		{"predict with two correct",
			"{text: any, explanation:", "{text: any, correct: true, explanation:",
			[]string{"go/go.yaml", "go.vars", "step 2", "exactly one correct", "2"}},
		{"predict with no correct",
			"{text: string, correct: true}", "{text: string}",
			[]string{"go.vars", "step 2", "exactly one correct", "0"}},
		{"predict with one option",
			"              - {text: any, explanation: 'The := operator infers the type of the value.'}\n", "",
			[]string{"go.vars", "step 2", "at least 2 options"}},
		{"predict without prompt",
			"prompt: What type does s get?", "prompt: \"\"",
			[]string{"go.vars", "step 2", "prompt"}},
		{"step without text",
			"        - text: var n int declares n with the zero value 0.\n", "        - text: \"\"\n",
			[]string{"go.vars", "step 1", "no text"}},
		{"code too long",
			"        code: |\n          var n int\n          s := \"hi\"\n          fmt.Println(n, s)\n", longCode,
			[]string{"go/go.yaml", "go.vars", "16 lines", "15"}},
		{"example without code",
			"        code: |\n          var n int\n          s := \"hi\"\n          fmt.Println(n, s)\n", "",
			[]string{"go.vars", "example", "no code"}},
		{"no intro",
			"      intro: Go declares a variable and its type together.\n", "",
			[]string{"go.vars", "lesson", "intro"}},
		{"no takeaway",
			"      takeaway: Every variable has a type and starts at its zero value.\n", "",
			[]string{"go.vars", "lesson", "takeaway"}},
		{"unknown lesson field",
			"      intro:", "      intor:",
			[]string{"go/go.yaml", "intor"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(lessonYAML, c.old) {
				t.Fatalf("fixture has no %q", c.old)
			}
			msg := loadErr(t, map[string]string{"go/go.yaml": strings.Replace(lessonYAML, c.old, c.new, 1)})
			wantAll(t, msg, c.parts...)
		})
	}
}

func TestLoadLessonTooManyPredictOptions(t *testing.T) {
	var opts strings.Builder
	opts.WriteString("              - {text: o0, correct: true}\n")
	for i := 1; i < 10; i++ {
		opts.WriteString("              - {text: o" + string(rune('0'+i)) + "}\n")
	}
	bad := strings.Replace(lessonYAML,
		"              - {text: string, correct: true}\n              - {text: any, explanation: 'The := operator infers the type of the value.'}\n",
		opts.String(), 1)
	msg := loadErr(t, map[string]string{"go/go.yaml": bad})
	wantAll(t, msg, "go.vars", "step 2", "10 options", "at most 9")
}
