# Documentation

Everything here describes **planned** behaviour until the status says otherwise. Evidence lives in
[benchmarks](benchmarks/) and the [feasibility gates](feasibility.md).

## Reading order

1. [Requirements](requirements.md): what must be true, and for whom
2. [Architecture](architecture.md): how the system meets those requirements
3. [Decision records](adr/): why it is built this way, and what was rejected
4. [API contract](../packages/contracts/openapi.yaml): what the browser and API agree on
5. [Threat model](threat-model.md): what could go wrong deliberately
6. [Feasibility gates](feasibility.md): what must be proven first

## Standards

| Document | Standard | Answers |
|---|---|---|
| Requirements | Product requirements with testable acceptance criteria | What and why? |
| Architecture | [arc42](https://arc42.org/overview) structure with [C4 model](https://c4model.com/) diagrams | How does it fit together? |
| Decision records | [Architecture decision records](https://adr.github.io/) | Why this choice? |
| API contract | [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0) with [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) errors | What is the interface? |
| Threat model | [STRIDE](https://learn.microsoft.com/en-us/azure/security/develop/threat-modeling-tool-threats) categories | What could an attacker do? |
| Runbooks | Symptom, diagnosis, recovery, verification | What do I do when it breaks? |
| Changelog | [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) | What changed in each release? |

## Keeping documentation true

- Change the architecture document and the code in the same pull request.
- Never edit an accepted decision. Write a new record that supersedes it.
- Diagrams are Mermaid text, so they are versioned and reviewed like code.
- Mark anything unproven as planned. Move it to fact only with evidence.
