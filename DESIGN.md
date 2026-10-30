# Oolong design

This document describes the implementation in this checkout. [README.md](README.md) is the entry point; [core architecture](docs/core/architecture.md) explains the runtime and scene, and [prior art](docs/decisions/prior-art.md) records the primary sources behind the design decisions.

## Goals

Oolong supports terminal interfaces whose content, interaction and external work have different lifetimes. It aims for predictable reading and input for users, composable contracts for developers, and explicit identities and outcomes for agents.

The design has one rule that applies at every level: every fact has one owner that can advance it. Another object may encode, cache or project that fact, but may not create a competing transition. The public API and the physical directories follow the same hierarchy: core primitives, base behavior, kit appearance and optional extensions.

## Ownership map

| Fact | Advancing owner | Read-only or bounded projections |
| --- | --- | --- |
| Attached identity, focus, modal eligibility and pointer capture | `scene.Root` | Elements, targets, semantic observations and written frame history |
| Editable text, revision and undo history | `base.EditSession` | Editor layouts, selections, rendered cells and change maps |
| Streamed source, EOF and accepted preparation | `content.StreamDocument` | Immutable source snapshots and renderer results |
| Retained conversation references and reading policy | `base.Transcript` | Transcript layout, logical reading anchor, selection rectangles and skin |
| Publication admission and pending responsibilities | `publication.Publisher` | Frozen content, batches and read-only receipts |
| Encoded output admission and writer completion | `term.Writer` | Queued and written sequence watermarks |
| Runtime work delivery and attachment lifetime | `program.Runtime` scopes | Worker result and settlement observations |
| Application theme palette | `kit.ThemeOwner` | Immutable scopes, instance overrides and resolved palette snapshots |

An owner boundary is justified by its guarantee, not by how many implementations currently exist. No compatibility layer copies an old model alongside the new owner.

## Core, base and kit

Core provides terminal-neutral values, cell drawing, Unicode geometry, scene attachment, typed actions and runtime composition. It cannot import base or kit. Base composes these primitives into editing, forms, choices, overlays, reading and virtualization. Kit fills appearance slots on those existing behavioral elements and combines components through scene groups.

A skin shares the same element identity and controller as its base behavior. It cannot create a second route by wrapping Draw and Handle methods. Focusability and semantic availability belong to the scene and behavior; a color token cannot decide whether an action is allowed. Application, subtree and instance theme values resolve through immutable scopes from one mutable palette owner.

Extensions consume small core content or host contracts. Markdown, code highlighting, mathematics and diagrams are not implemented as branches inside core. Each dependency-heavy extension is independently versioned.

## Frame and input transactions

Scene preparation produces an immutable candidate. Geometry, semantic observations and per-component snapshots become current only after compositor admission. Aborting a candidate keeps the prior complete projection. Source and controller changes already made on the owner turn remain facts; a frame abort does not undo them.

A controller stages its geometry only through the root that mounts its actual Element. Read-only composition borrows exclude model changes, attachment changes and snapshot staging or refinement. An unsettled candidate excludes a new read-only borrow, so a previously admitted Commit never needs to invoke callbacks or acquire another permission. The same scene phase owns these rules before and during publication capture; components do not keep separate capture locks.

Pointer events use the projection associated with their arrival or written frame identity. A resize invalidates incompatible geometry, and written history is bounded. Newer geometry must not reinterpret an old cell coordinate. Capture, focus and modal routing are owned centrally by the root rather than by independent component containers.

The runtime composes scene candidates, publication candidates and exact terminal payloads before committing their corresponding owners. A refused writer admission leaves candidates retryable. Physical writer completion advances the receipt separately from frame acceptance. Callbacks are not invoked halfway through those commits.

## Text and editing

`core/text.Layout` owns the mapping between logical text positions and cell geometry. Grapheme clusters, soft wraps and wide cells are resolved once through shared layout atoms. Editors share an `EditSession`; views retain their own reading and composition policy without owning another editable string or undo stack.

Transcript selection uses content identity, source revision and logical positions. A renderer's meaningful rows provide its copy-text interpretation. A width change preserves selection only while that logical text projection remains equivalent. Arbitrary rich-format source maps are not inferred from painted cells; a renderer must provide any stronger semantic mapping it promises.

## Source and publication lifetimes

A transport chunk is not a syntax boundary. StreamDocument stores bounded source, advances its revision and EOF fact, and accepts preparation only for the matching source identity, revision and finality. A preview can be replaced by later preparation. End-of-source causes a final revision even if no extra bytes arrived.

Markdown preparation uses the entire immutable source snapshot. Later reference definitions and syntax completion can revise earlier preview rows. Final rendering is tested against whole-source rendering across arbitrary chunk partitions. This deliberately removes the old prefix publication protocol and its chunk-dependent language semantics.

Transcript adds a source subscription once. Its retention policy decides when a complete leading reference can leave the interactive surface. `KeepCompleted` retains a bounded recent run; `MaxItems` limits active references. Publication capacity refusal keeps the content and subscriptions available; deferred publisher progress resumes admission. Retirement releases source, body and subscription references while preserving the item identity and receipt.

Publisher captures immutable display atoms with explicit item, cell and storage budgets. Native terminal paint is refused for permanent history because a stable native region cannot be promised there. The compositor, which knows the actual encoding and viewport, checks encoded-payload limits. Accepted, Written, Failed and UnknownPartial remain different receipt outcomes.

## Work, actions and automation

Background work is scoped to an application or attached element and returns through typed owner delivery. Worker cancellation does not imply that owner delivery happened; completed work can still become obsolete before application. Shutdown closes admission before draining accepted responsibilities and settling resources.

A typed scene action describes availability and invokes the same controller command used by input. Automation observes bounded snapshots and resolves locators on the owner turn. Duplicate matches, stale targets, truncated observations and unsupported actions are explicit results. Observing the interface does not mount hidden content or mutate focus.

## History and resource limits

Retained interactive content, permanent terminal history and application archives are separate lifetimes. The terminal owns already written history. An application may retain a searchable archive and supply a virtual source; core does not silently duplicate all published source into a second archive.

VirtualWindow retains a bounded key index and height cache and materializes only its active range. Unknown heights use explicit estimates. Exactness is observable, and source revision changes invalidate stale measurements. Key indexing and prefix calculation still take O(N) work within the configured item budget; this implementation does not claim that every history operation is proportional only to visible rows.

Budgets exist at each actual stage: ingress bytes, source bytes, retained references, materialized nodes, captured cells and storage, pending publication, encoded frames and writer queues. A limit produces refusal or backpressure at its owner. It does not silently discard content or switch a format's semantics.

## Verification and remaining limits

Tests cover scene atomicity, stale input, logical text mapping, model reentry, source partition equivalence, retained-heap bounds, publication retries, partial writes, resize behavior and native-resource lifetime. Architecture checks derive imports from Go package facts. Documentation tests compile the tutorial, check translations and resolve repository links. [CONTRIBUTING.md](CONTRIBUTING.md) lists the gates.

This version supplies composition primitives rather than a persistent conversation database, a universal rich-format source map or automatic materialization of every offscreen automation target. Those features require a concrete data owner, storage and loading policy, and useful workload evidence. [ROADMAP.md](ROADMAP.md) describes their acceptance criteria.
