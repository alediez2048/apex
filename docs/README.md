# Documentation Index

Mobile Check Deposit System — Apex Fintech Solutions

---

## Directory Structure

```
docs/
├── README.md                                  ← You are here
├── requirements/
│   └── project_brief.md                       ← The original challenge brief
├── architecture.md                            ← System diagram & data flow
├── decision_log.md                            ← 10 ADRs in standard format
└── demo/
    └── demo_guide.md                          ← Demo script, presenter notes & troubleshooting
```

---

## By Category

### Requirements

Documents defining **what** to build.

| Document | Description | Audience |
|---|---|---|
| [Project Brief](requirements/project_brief.md) | The original challenge brief: functional requirements, success criteria and deliverables. | All |

### Architecture

Documents defining **how** to build it and **why** each decision was made.

| Document | Description | Audience |
|---|---|---|
| [architecture.md](architecture.md) | System diagram, service boundaries, data flow. | Engineers, Reviewers |
| [decision_log.md](decision_log.md) | 10 ADRs covering language, data store, vendor stub, settlement, state machine, currency, risk scoring, API, concurrency, and pipeline. | Reviewers |

### Demo

Documents for **presenting** the finished system.

| Document | Description | Audience |
|---|---|---|
| [Demo Guide](demo/demo_guide.md) | 6-act demo script (happy path, vendor rejections, business rules, operator review, returns, invariant verification). Includes architecture walkthrough, key file map, presenter talking points and troubleshooting. | Presenter, Reviewers |

---

## Quick Reference

| I need to... | Go to |
|---|---|
| Understand the project requirements | [Project Brief](requirements/project_brief.md) |
| Understand why a design decision was made | [Decision Log](decision_log.md) |
| Prepare for the demo | [Demo Guide](demo/demo_guide.md) |

---

## Document Lifecycle

| Phase | Active Documents |
|---|---|
| Planning | Project Brief, PRD, Architectural Blueprint |
| Implementation | Development Log (updated per ticket) |
| Pre-Submission | Demo Guide, architecture.md, decision_log.md, SUBMISSION.md |
