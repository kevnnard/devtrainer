# Study plan

Goal: interview-ready across programming, architecture and tooling, and ready for AWS Solutions Architect
Associate (SAA-C03). About 26 weeks, starting from the beginning.

## Daily routine (5 days a week, about 30 minutes)

1. **Reviews first (15–20 min).** Open `devtrainer` and start the session: due reviews come first, mixed across tracks. Do all of them; skipping reviews is how forgetting wins.
2. **New concepts (10 min).** Up to 3 new concepts a day (`--new` changes it). Fewer on heavy review days.
3. **Read every explanation you got wrong.** Press `e` when the explanation is not enough and the tutor explains deeper. When a code exercise fails, read the real error before trying again.
4. **Build something small (optional, 30+ min).** Apply the day's concept in real code: a script, a Dockerfile, a SQL query, a Terraform-free AWS console exercise.

Code exercises come up as part of the session, like any other item. Run them only with toolchains you have
installed (`devtrainer validate` lists them); see the [README](../README.md#code-exercises).

Weekly: open **Stats**, pick your top two misconceptions and your weakest concept, and study them on purpose.
When every concept of a section is at least "learning", take its exam from Tracks (`x`); sections exist in the
Go, Python, JavaScript and SQL tracks. Its answers are scheduled like any other review.

## Phases

| Phase | Weeks | Tracks | Done when |
|---|---|---|---|
| Base | 1–4 | logic, git, linux | Every base concept is at least in review state |
| Language | 5–10 | go (main), python, typescript | Go concepts mastered; you can solve easy interview problems in Go |
| Infra | 11–14 | docker, sql | You can containerize an app and write joins, aggregates and transactions without notes |
| Design | 15–20 | architecture | You can defend a design: coupling, layers, caching, CAP, idempotency, queues |
| Cloud | 21–26 | aws (SAA-C03 domains: IAM, VPC, EC2, S3, databases, scaling, well-architected) | AWS track mastered, then official practice exams |

Phases overlap: the order of tracks in devtrainer follows this path, and prerequisites unlock concepts of the
next phase while older ones keep coming back as reviews.

### Other languages

JavaScript, Java (21), C# (14 / .NET 10) and Rust (2021/2024 editions) are available as extra tracks. They are
not part of the 26-week plan: add one when you need it, for example during or after the Language phase, and
lower `--new` if the daily load grows too much.

## Mastery rule

A concept is mastered after **4 correct answers in its last 5, on at least 2 different days**. Moving on early
is fine; reviews will catch what you forget.

## Growing the content

When a track feels too small, draft new items with the tutor and review them before adding them:

```sh
devtrainer draft aws "S3 storage classes and lifecycle rules"
# writes drafts/aws-s3-storage-classes-and-lifecycle-rules.yaml
# check it, then merge the concepts into content/aws/aws.yaml
devtrainer validate
go install ./cmd/devtrainer   # rebuild so the binary embeds the new content
```
