# Stash NG Server — Unraid Companion context glossary

> **Scope:** this file is *narrow* — Unraid deployment, audit, and host-integration terminology only.
> It is the **server repo's** context glossary (mapped from [`../CONTEXT-MAP.md`](../CONTEXT-MAP.md)).
> For **product-wide** vocabulary (Scene, Folder, `serverCapabilities`, `deletedSince`, `deviceBus`,
> `moveFolder`, tombstone, NG-only) see the shared glossary [`../docs/glossary.md`](../docs/glossary.md);
> deep contract detail is in [`../docs/api/ng-contract.md`](../docs/api/ng-contract.md). For live
> ecosystem status, see [`../docs/agent-handoff/README.md`](../docs/agent-handoff/README.md).

This file defines the **ubiquitous language for the Unraid Companion** (the host-resident
Unraid integration for the Stash NG Container) — its findings, audit modes, safety
boundaries, and deployment terms — so host-side conversations use the same words. Unraid
Companion V1 has shipped; the terms below are the domain language it was built on.

## Language

**Stash NG**:
The NG-only Stash ecosystem: the forked server plus the native clients that speak its GraphQL contract.
_Avoid_: upstream Stash, vanilla Stash, stock Stash

**Stash NG Server**:
The server-side source of truth for the library, media metadata, and the GraphQL contract consumed by the clients.
_Avoid_: backend app, web UI, Docker app

**Stash NG Container**:
The Docker-packaged runtime for the Stash NG Server on a host such as Unraid.
_Avoid_: Unraid plugin, server plugin

**Unraid Companion Plugin**:
A host-resident Unraid integration that assists the Stash NG Container with Unraid-specific deployment, safety, storage, hardware, or operational concerns.
_Avoid_: Stash plugin, Docker template, container

**Managed Target**:
The Stash NG Container instance that the Unraid Companion Plugin is allowed to inspect and assist.
_Avoid_: stock Stash, upstream Stash, any Stash container

**Candidate Container**:
A Docker container that appears to be related to Stash based on name, image, template, ports, or mounts before the companion proves it is the Managed Target.
_Avoid_: managed target, verified server

**Identity Mismatch**:
A Blocking Finding where host-level container identity and server-reported Stash NG identity disagree, or where multiple candidates share the same appdata/database.
_Avoid_: warning, detection ambiguity

**Migration Source**:
An existing stock Stash installation that the Unraid Companion Plugin may detect only to support a deliberate cutover to Stash NG.
_Avoid_: managed target, fallback server

**Stash Plugin**:
An extension loaded by the Stash NG Server through Stash's own plugin system.
_Avoid_: Unraid plugin, host plugin, Docker template

**Stash Plugin Inventory**:
A narrow Info Finding that reports whether Stash in-app plugins exist in appdata and how many are detected.
_Avoid_: plugin security audit, script review

**Docker Template**:
An Unraid container definition that describes how to run the Stash NG Container.
_Avoid_: Unraid Companion Plugin, Stash plugin

**Template Drift**:
An Audit Finding where a live Candidate Container differs from the expected Stash NG Docker Template in a way that may affect safety, performance, or supportability.
_Avoid_: automatic template rewrite, container mismatch

**Unraid Packaging Area**:
The Stash NG Server repo area that owns Unraid deployment artifacts, including the Docker Template and the future Unraid Companion Plugin source.
_Avoid_: Stash plugin directory, client repo

**Traditional Unraid Plugin**:
A host-installed Unraid plugin that runs in the Unraid management environment rather than as a Docker sidecar.
_Avoid_: sidecar container, Stash plugin, Docker template

**Unraid-Native Page**:
A companion UI implemented inside Unraid's existing management interface rather than as a separate web app or service.
_Avoid_: separate web UI, daemon UI, Stash web UI

**Fixed Audit Helper**:
A narrowly scoped helper invoked by the Unraid-Native Page to perform a named audit or backup operation.
_Avoid_: command runner, shell console, daemon

**Deployment Safety**:
The set of host-level checks and user-confirmed actions that reduce the risk of one-way schema migration, appdata mistakes, conflicting containers, missing backups, or unsafe storage mappings.
_Avoid_: migration automation, setup wizard

**Host-Aware Performance Check**:
An Unraid-specific observation about storage, Docker, GPU, CPU, memory, or scheduled host activity that may affect Stash NG performance.
_Avoid_: generic optimization, app tuning

**Share Policy Finding**:
An Audit Finding based on Unraid share settings such as cache policy, mover behavior, array/parity placement, or related storage layout.
_Avoid_: automatic share rewrite, generic path warning

**Host Fact**:
Information about the Unraid host, Docker runtime, storage shares, device mappings, scheduled host activity, or backup files that exists outside the Stash NG Server.
_Avoid_: library truth, server state

**Server Fact**:
Information reported by the Stash NG Server through its API, such as capabilities, configured paths, job status, or health.
_Avoid_: host fact, Docker state

**Config File Fact**:
Information read from the Stash NG `config.yml` during a No-Credential Audit for deployment and safety checks.
_Avoid_: library truth, running server truth

**Library Truth**:
The Stash NG Server's authoritative view of the media library and metadata.
_Avoid_: SQLite scrape, filesystem guess, Docker inspection

**Control Plane**:
The place where the user reviews findings and deliberately triggers companion actions.
_Avoid_: Stash web UI, native clients

**Host Health Summary**:
A read-only summary of Unraid Companion Plugin findings that may be shown outside Unraid without exposing host controls.
_Avoid_: control plane, management UI

**Container Audit**:
The Unraid Companion Plugin's read-first evaluation of an existing Stash NG Container and its host environment.
_Avoid_: installer, updater, migration

**Guided Install**:
A later companion workflow that creates or updates the Stash NG Container only through explicit user-confirmed steps.
_Avoid_: auto-install, background update

**Guided Update**:
A later companion workflow that updates an existing Managed Target through explicit user-confirmed steps, including backup, container/template change, restart, verification, and report.
_Avoid_: auto-update, background upgrade

**Preferred Unraid Flow**:
The future state where Unraid users normally install or update Stash NG through the Unraid Companion Plugin after the V1 audit model has proven safe.
_Avoid_: V1 installer, automatic lifecycle manager

**No-Credential Audit**:
A Container Audit mode that uses only Host Facts and does not require access to the Stash NG Server API.
_Avoid_: anonymous server probe, limited login

**Server-Aware Audit**:
An optional Container Audit mode that uses stored Stash NG credentials to include Server Facts.
_Avoid_: required login, direct database access

**Ephemeral API Key**:
A Stash NG API key provided for a single Server-Aware Audit and not stored after the audit completes.
_Avoid_: saved credential, plugin secret

**Stored API Key**:
An optional Stash NG API key retained by the Unraid Companion Plugin for future Server-Aware Audits.
_Avoid_: default credential, exported secret

**Reversible Remediation**:
A user-triggered companion action that can be undone without changing library truth, migrating the database, moving media, or replacing the running server.
_Avoid_: migration, image rebuild, config rewrite

**Appdata Backup**:
A timestamped backup artifact of the configured Stash NG appdata/database area created by the Unraid Companion Plugin as a user-triggered safety action.
_Avoid_: migration backup, restore point, server snapshot

**Critical Appdata Backup**:
An Appdata Backup that includes the Stash NG database, configuration, and plugin/scraper state needed for rollback or migration safety, while excluding generated, cache, transcode, and other large derived artifacts by default.
_Avoid_: full media backup, generated backup, cache backup

**Risky Remediation**:
A companion action that changes appdata, rewrites Stash configuration, migrates the database, rebuilds the image, restarts the container, or touches media.
_Avoid_: quick fix, automatic repair

**Audit Finding**:
A specific Container Audit result with evidence, severity, and a recommended user action.
_Avoid_: warning, guess, optimization tip

**Local Evidence**:
The exact host paths, container settings, share names, and server facts shown inside the authenticated Unraid Control Plane for an Audit Finding.
_Avoid_: export-safe evidence, public report

**Redacted Audit Export**:
An exported Container Audit report that masks secrets and sensitive host or library details by default.
_Avoid_: local evidence dump, debug archive

**V1 Audit Set**:
The first must-have group of Container Audit checks that proves the Unraid Companion Plugin is useful before broader installation or update workflows exist.
_Avoid_: full health system, final checklist

**Finding Severity**:
The domain-specific label that explains why an Audit Finding matters: Blocking, Risk, Performance, or Info.
_Avoid_: generic warning, generic error

**Blocking Finding**:
An Audit Finding that means the user should not proceed with cutover, update, or risky action until it is resolved.
_Avoid_: fatal error, hard failure

**Risk Finding**:
An Audit Finding about data loss, privacy, host safety, or security exposure.
_Avoid_: warning, concern

**Performance Finding**:
An Audit Finding about likely slowdown or resource contention.
_Avoid_: optimization tip, speed warning

**Info Finding**:
An Audit Finding that records neutral evidence without asking the user to change anything.
_Avoid_: success, pass

**Manual Audit**:
A Container Audit run that starts when the user opens the companion page or explicitly clicks a run action.
_Avoid_: background monitor, daemon

**Scheduled Audit**:
A future Container Audit mode that runs periodically and reports only changed high-severity findings.
_Avoid_: continuous monitoring, telemetry

**Local-Only Operation**:
The companion posture where audit work stays on the Unraid host and, when credentials are provided, the local Stash NG Server.
_Avoid_: telemetry, phone-home, external update check

**Host Security Boundary**:
The rule that the Unraid Companion Plugin must not expand Unraid's attack surface beyond the minimum needed to audit and assist Stash NG.
_Avoid_: convenience access, broad host control

**Privileged Host Context**:
The Unraid environment in which a Traditional Unraid Plugin can observe or affect host-level state.
_Avoid_: sandbox, app context

**V1 Security Boundary**:
The first-version Host Security Boundary: no new network listener, no external calls, no Docker socket proxy, no arbitrary command runner, and no automatic writes beyond companion config, reports, and explicit Appdata Backup artifacts.
_Avoid_: admin helper, remote agent, automation daemon

**General Command Surface**:
Any interface that accepts arbitrary commands, command fragments, shell arguments, scripts, or remote instructions for execution on the Unraid host.
_Avoid_: helper, audit action

## Example Dialogue

Dev: "Should the Unraid Companion Plugin move files?"

Domain expert: "No. File-relocating library operations belong to the Stash NG Server and are exposed through its contract. The Unraid Companion Plugin may warn about share layout or mount risk, but it is not the library authority."

Dev: "Can the Docker Template solve migration safety?"

Domain expert: "Only partially. The Docker Template can set defaults, but an Unraid Companion Plugin can inspect the host, detect the existing container and appdata, and guide a safer cutover."

Dev: "Should V1 compare the live container with the Docker Template?"

Domain expert: "Yes. Template Drift should be reported as an Audit Finding when it affects safety, performance, or supportability, but V1 should not rewrite the template."

Dev: "Is a container named stash-ng automatically the managed target?"

Domain expert: "No. It is only a Candidate Container until the companion checks mounts and, when credentials are available, Stash NG server identity. Any Identity Mismatch is Blocking."

Dev: "Where should the companion source live?"

Domain expert: "In the Unraid Packaging Area of the Stash NG Server repo, close to the Docker Template, because the companion audits that deployment contract."

Dev: "Should V1 run a separate service?"

Domain expert: "No. V1 should be an Unraid-Native Page calling Fixed Audit Helpers. It should not introduce a daemon, listener, Node service, Go service, or General Command Surface."

Dev: "Should the companion run as a sidecar container?"

Domain expert: "No. The companion needs transparent access to host-native Unraid state, so V1 should be a Traditional Unraid Plugin. Stash NG itself remains the containerized server."

Dev: "Should the companion change generated paths automatically if it finds them on slow storage?"

Domain expert: "No. That is a Deployment Safety concern. It should explain the risk and offer an explicit user-triggered action."

Dev: "Should the companion understand Unraid cache and mover policy?"

Domain expert: "Yes. Those are Share Policy Finding inputs. V1 should explain the performance or safety risk, but it should not rewrite share settings."

Dev: "Can the companion read stash-go.sqlite to count scenes faster?"

Domain expert: "No. Library Truth belongs to the Stash NG Server. The companion can inspect Host Facts directly, but Server Facts must come from the server API."

Dev: "Can the companion parse config.yml without a Stash API key?"

Domain expert: "Yes, but only as a Config File Fact for deployment and safety checks. If the API is available, Server Facts from the running process take precedence."

Dev: "Should Stash NG contain buttons that modify Unraid?"

Domain expert: "No. The Control Plane belongs in Unraid. Stash NG may show a Host Health Summary later, but host actions stay in the companion UI."

Dev: "Should the companion build and replace the container before it can audit anything?"

Domain expert: "No. Container Audit comes first. Guided Install is a later workflow once the companion can already explain the host risks accurately."

Dev: "Should the companion always stay audit-only?"

Domain expert: "No. The Preferred Unraid Flow can eventually include Guided Install and Guided Update, but only after Container Audit, identity proofing, backup, and the Host Security Boundary are proven."

Dev: "Does the companion need a Stash API key before it can be useful?"

Domain expert: "No. A No-Credential Audit should still catch host risks. A Server-Aware Audit adds server facts only after the user chooses to provide credentials."

Dev: "Should the companion save my API key by default?"

Domain expert: "No. Use an Ephemeral API Key first. A Stored API Key is an opt-in convenience and must be masked, restricted, deletable, and excluded from audit exports."

Dev: "Can V1 fix every issue it finds?"

Domain expert: "No. V1 may offer Reversible Remediation, such as an audit export or appdata backup, but Risky Remediation waits until the companion has a proven audit model."

Dev: "Can V1 create a backup before I experiment?"

Domain expert: "Yes. An Appdata Backup is allowed as a narrow user-triggered safety action, but V1 should not stop containers, migrate databases, or restore backups automatically."

Dev: "Should V1 back up generated previews and cache?"

Domain expert: "No. V1 should create a Critical Appdata Backup by default: database, config, plugins, and scrapers, with generated, cache, transcodes, and blobs excluded unless a later design explicitly expands scope."

Dev: "Should the companion judge whether Stash plugins are safe?"

Domain expert: "No. V1 may provide a Stash Plugin Inventory, but in-app plugin code review is a separate security product."

Dev: "What makes an audit result trustworthy?"

Domain expert: "It must be an Audit Finding: a concrete result with evidence, severity, and a recommended action. The V1 Audit Set should stay small enough that each finding is specific and defensible."

Dev: "Should exported reports show full media paths?"

Domain expert: "No. The Control Plane can show Local Evidence, but exported reports should be Redacted Audit Exports by default so they are safe to share."

Dev: "What severities should findings use?"

Domain expert: "Use Finding Severity: Blocking for do-not-proceed issues, Risk for data-loss/privacy/security concerns, Performance for likely slowdown, and Info for neutral evidence."

Dev: "Should the companion watch the server all day?"

Domain expert: "No. V1 uses Manual Audit and stores the latest report. Scheduled Audit can come later if the findings are stable enough to justify notifications."

Dev: "Can the companion send anonymous usage or health metrics?"

Domain expert: "No. V1 uses Local-Only Operation. It may inspect Unraid, Docker, files, and the local Stash NG API, but it should not make external requests by default."

Dev: "Can the companion expose broad host controls because it runs inside Unraid?"

Domain expert: "No. The Host Security Boundary is strict because a Traditional Unraid Plugin runs in a Privileged Host Context. V1 should minimize permissions, network exposure, and writable actions."

Dev: "What is the V1 security line?"

Domain expert: "The V1 Security Boundary allows local reads, optional local Stash NG API calls, and tightly scoped companion writes. It does not allow a new listener, external calls, Docker socket proxying, arbitrary command execution, or automatic host mutation."
