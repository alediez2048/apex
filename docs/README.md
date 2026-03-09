# Documentation Index

Mobile Check Deposit System — Apex Fintech Services

---

## Directory Structure

```
docs/
├── README.md                                  ← You are here
├── requirements/
│   ├── project_brief.md                       ← Original project spec & evaluation rubric
│   └── PRD.md                                 ← Product Requirements Document (v1.1)
├── architecture/
│   ├── architectural_blueprint.md             ← 30 architectural decisions across 3 rounds
│   ├── architecture.md                        ← System diagram & data flow (created during TICKET-015)
│   └── decision_log.md                        ← 10 ADRs in standard format (created during TICKET-015)
├── development/
│   └── devlog.md                              ← Development log & ticket tracker
└── demo/
    └── demo_guide.md                          ← Demo script, presenter notes & troubleshooting
```

---

## By Category

### Requirements

Documents defining **what** to build and how it will be evaluated.

| Document | Description | Audience |
|---|---|---|
| [Project Brief](requirements/project_brief.md) | Original Apex Fintech challenger project spec. Functional requirements, success criteria, evaluation rubric (100 pts), deliverables checklist. | All |
| [PRD v1.1](requirements/PRD.md) | Comprehensive product requirements derived from 30 architectural interviews. Personas, user stories, functional requirements (P0/P1/P2), user flows, technical architecture, implementation tickets, rubric traceability matrix. | All |

### Architecture

Documents defining **how** to build it and **why** each decision was made.

| Document | Description | Audience |
|---|---|---|
| [Architectural Blueprint](architecture/architectural_blueprint.md) | 30 critical decisions across 3 rounds (Foundations, Implementation, Operations). Each evaluated through 3 strategies with pros/cons and a justified winner. | Engineers, Reviewers |
| architecture.md | System diagram, service boundaries, data flow. *(Created during TICKET-015)* | Engineers, Reviewers |
| decision_log.md | 10 ADRs covering language, data store, vendor stub, settlement, state machine, currency, risk scoring, API, concurrency, and pipeline. *(Created during TICKET-015)* | Reviewers |

### Development

Documents tracking **progress** during implementation.

| Document | Description | Audience |
|---|---|---|
| [Development Log](development/devlog.md) | Ticket-by-ticket execution tracker. 15 tickets across 3 phases (~131h). Status, blockers, dependencies, risks & mitigations. | Developer |

### Demo

Documents for **presenting** the finished system.

| Document | Description | Audience |
|---|---|---|
| [Demo Guide](demo/demo_guide.md) | 6-act demo script (happy path, vendor rejections, business rules, operator review, returns, invariant verification). Includes architecture walkthrough, key file map, presenter talking points, rubric alignment checklist, and troubleshooting. | Presenter, Reviewers |

---

## Quick Reference

| I need to... | Go to |
|---|---|
| Understand the project requirements | [Project Brief](requirements/project_brief.md) |
| See the full PRD with user stories and tickets | [PRD](requirements/PRD.md) |
| Understand why a design decision was made | [Architectural Blueprint](architecture/architectural_blueprint.md) |
| Check ticket status and blockers | [Development Log](development/devlog.md) |
| Prepare for the demo | [Demo Guide](demo/demo_guide.md) |
| See the evaluation rubric | [Project Brief](requirements/project_brief.md) § Evaluation Rubric |
| See rubric traceability | [PRD](requirements/PRD.md) § Appendix A |

---

## Document Lifecycle

| Phase | Active Documents |
|---|---|
| Planning | Project Brief, PRD, Architectural Blueprint |
| Implementation | Development Log (updated per ticket) |
| Pre-Submission | Demo Guide, architecture.md, decision_log.md, SUBMISSION.md |
