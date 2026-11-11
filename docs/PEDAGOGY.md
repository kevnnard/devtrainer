# Pedagogy

How devtrainer teaches, and the research behind each choice.

## Principles

| Principle | What devtrainer does | Evidence |
|---|---|---|
| Retrieval practice | Sessions are built on answering, not rereading: a short worked lesson introduces a new concept, then you practice by answering questions and writing code before you see the solution. | Carpenter, Pan & Butler, *The science of effective learning with spacing and retrieval practice*, Nature Reviews Psychology (2022) |
| Spacing | FSRS schedules each **concept** for the day you are about to forget it. | Same review; YeckehZaare, Resnick & Ericson, *A Spaced, Interleaved Retrieval Practice Tool that is Motivating and Effective*, ICER 2019 |
| Interleaving | Due reviews are mixed across tracks; two concepts from the same track never come in a row when it can be avoided. Section exams mix items from all the concepts of a section. | Systematic review of interleaving as a concept-learning strategy, Review of Education (2021) |
| Variation | A review shows a different item of the concept than last time, so you learn the idea, not the answer. | Distributed MCQ practice for novices: Zhang et al., iJET (2020) |
| Misconception feedback | Every wrong option names the misconception behind it and explains why it is wrong. Stats show your most frequent misconceptions. A failed code exercise shows the real compiler or runtime error. | Muldner, Jennings & Chiarelli, *A Review of Worked Examples in Programming Activities*, TOCE (2022) |
| Scaffolded difficulty | Item types go from recognizing (mcq) to tracing (output) to arranging (parsons) to diagnosing (bug) to producing (cloze, command, code). Some lesson steps show a real error and ask you to predict why it happens. | Ericson et al., *Parsons Problems and Beyond* (2022); Xie et al., *A theory of instruction for introductory programming skills* (2019) |
| Mastery and prerequisites | A concept unlocks only when its prerequisites were answered correctly; it is mastered after 4 correct of the last 5 on at least 2 days. A section exam opens when all its concepts are at least "learning" and passes at 80%. | Mastery learning; knowledge-graph sequencing |
| Honest grading | The grade comes from what you did: wrong = Again, hint or slow = Hard, correct = Good, fast = Easy. Code exercises: fail = Again, pass after a hint = Hard, pass = Good, or Easy within 90 s (run time not counted). No self-rating. | FSRS grades; avoiding metacognitive overconfidence |

## Limits

- Closed questions check concepts and reading code. Code exercises are small, single-file tasks graded by their output; neither replaces building real programs. Build projects alongside devtrainer.
- Code exercises grade output, not style or design: a solution that prints the right thing passes.
- FSRS predicts forgetting better than SM-2 (open-spaced-repetition/srs-benchmark), but no study shows it makes you learn more. It saves review time.
- AI drafts (`devtrainer draft`) are candidates. They are validated against the schema, but a human must check them for correctness before they move into `content/`.
