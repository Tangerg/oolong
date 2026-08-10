---
title: Oolong brand
description: Use Oolong's name, visual language, and streaming-first vocabulary consistently.
contentType: Reference
outline: deep
---

# Oolong brand

Language: English | [简体中文](zh/brand.md)

Status: brand foundation and naming guide. This document records what the name means,
how Oolong describes itself, and where brand language must stop. The
[architecture](architecture.md) remains the source of truth for technical contracts.

## 1. Brand core

Oolong is calm infrastructure for interfaces through which output keeps arriving.

Its central promise is:

> Streams can be long. The live interface stays bounded.

Completed output moves forward into terminal-owned history. Only work that is open,
changing, or deliberately interactive remains live. The brand should make that motion
feel natural without turning the implementation into a metaphor.

Oolong should feel:

- **flowing**, because data arrives incrementally;
- **bounded**, because session age does not become live UI cost;
- **quiet**, because an idle interface does no work and writes no bytes;
- **warm**, because it is made for people using terminals, not for a rendering demo;
- **precise**, because ownership, ordering, failure, and dependency direction are
  explicit;
- **composable**, because applications assemble behavior instead of entering a closed
  framework.

## 2. The name

### 2.1 Official spelling

The product name is **Oolong** in prose and `oolong` in repository names, module paths,
commands, and wordmarks that use lowercase.

`ooloog` is not an alternate spelling. Its repeated circles have visual energy, but it
looks like a typo, loses the real-word association, and makes pronunciation, search,
and recall harder. A wordmark may make the letters flow into one another, but the `n`
must remain recognizably an `n`.

### 2.2 A left-to-right flow

The lowercase word has a useful visual reading:

```text
oo                 l                    o                    ng
upstream flow  ->  processing boundary -> bounded live view -> terminal and history
```

- The first `oo` suggests successive chunks arriving from upstream.
- The vertical `l` suggests a narrow processing and ownership boundary.
- The following `o` suggests the one bounded, still-live presentation.
- The final `ng` carries the word onward toward the terminal surface; the descender of
  `g` may suggest output continuing below the viewport into scrollback.

This is a visual story, not an acronym and not a technical state machine. It should
guide a wordmark or a small motion study without being forced into every explanation of
the project.

### 2.3 `oo + long`

The word also reads naturally as `oo` plus `long`: a flow may be long-running without
making the active interface grow for just as long. This is the strongest connection
between the name and the streaming-first architecture.

The claim is architectural, not decorative. Oolong earns it only while committed
payload leaves the component graph, incremental transforms retain short open tails,
and work is bounded by the live interface rather than session age.

### 2.4 The tea association

Oolong tea gives the name warmth and memorability. Water passes through leaves and
draws out content over time, which is a gentle secondary image for incremental
transformation. It is not the primary technical explanation, and it must not become a
taxonomy for the codebase.

## 3. Positioning

### 3.1 Official descriptor

Use this complete technical descriptor:

> **Oolong is a streaming-first terminal UI substrate for Go.**

Use this shorter form where space is limited:

> **Streaming-first terminal UI for Go.**

The word **substrate** is intentional. Oolong supplies terminal, publication,
interaction, layout, text, headless behavior, and polished component foundations. It
does not require every application to adopt one product grammar or one declarative
authoring model.

### 3.2 Message order

When introducing Oolong, explain it in this order:

1. Output can keep arriving incrementally.
2. Finished output becomes terminal scrollback instead of permanent live UI state.
3. One goroutine owns ordinary mutable domain objects.
4. Headless behavior and polished appearance compose in separate layers.
5. Lower packages remain general and dependencies point one way.

Do not lead with a widget catalogue, a comparison to another framework, or an internal
rendering technique. Those are consequences and implementation choices. The lifetime
of output is the differentiator.

### 3.3 Claims and their proof

| brand claim | technical truth required |
| --- | --- |
| streaming-first | incremental ingestion and transformation; explicit ordering, batching, backpressure, cancellation, and finalization |
| bounded | committed payload and per-item placement leave the live graph; memory and resize cost do not follow session age |
| terminal-native | completed output is published into scrollback and is not retained merely so the program can redraw it |
| composable | headless behavior, polished `kit` components, and product grammar remain distinct and depend one way |
| Go-native | concrete domain objects, consumer-defined interfaces, useful zero values, explicit errors, and no framework-shaped service locator |
| quiet | no unconditional frame clock, no repeated idle bytes, and explicit timer lifetimes |

Marketing must not outrun these executable truths. If one is temporarily false, fix
the implementation or narrow the claim; do not explain the contradiction away.

## 4. Component-story language

The public component story has three application-facing levels:

```text
headless behavior -> polished kit component -> application product grammar
```

- **Headless** means complete interaction behavior and typed semantic state without a
  mandated appearance. Base UI and Radix UI are useful references.
- **Kit** means an ergonomic, attractive, themeable composition over headless parts.
  This is the shadcn lesson Oolong adopts: strong defaults and a shorter path to a
  finished interface.
- **Application** means the product-specific workflow and language that a shared
  library must not absorb.

Oolong does not adopt source copying as its component distribution model. `kit` is
ordinary imported, versioned Go code. A polished default remains open to composition:
callers can reach the underlying controller and semantic parts when they need a
different arrangement.

## 5. Voice and writing

Oolong writes like a thoughtful infrastructure library: direct, calm, concrete, and
slightly warm.

### 5.1 Prefer

- lead with what a type or decision enables;
- explain why a boundary exists, especially when the signature cannot;
- use ordinary words before architecture jargon;
- name ownership, lifetime, ordering, and failure explicitly;
- distinguish current behavior, required direction, and optional future work;
- use small examples that show the normal path linearly;
- make performance claims in terms of complexity or measurements.

### 5.2 Avoid

- hype such as "blazing fast", "revolutionary", or "magic";
- defining Oolong mainly by dismissing another project;
- saying "reactive" or "declarative" without naming the actual data flow;
- calling a policy an invariant when no failure or enforcement can be described;
- tea puns in serious API documentation;
- presenting optional future layers as if they already exist.

Warmth belongs in rhythm, examples, illustrations, and the name itself. Precision still
wins whenever a reader must make a technical decision.

## 6. Visual direction

### 6.1 Wordmark

The preferred wordmark is lowercase `oolong`. It should remain immediately readable at
terminal-sized resolutions.

The most useful visual features are:

- rounded `o` forms that suggest successive units in a flow;
- a clear vertical `l` that can act as a processing or ownership boundary;
- continuous left-to-right spacing rather than a sealed badge;
- an unmistakable `n`, preserving the spelling;
- a `g` whose descender may imply continuation into history.

The letter story should be discoverable, not diagrammed into the logo. Readability
comes before cleverness.

### 6.2 Motion

If the brand is animated, motion should express the architecture:

1. small units arrive from the left;
2. they pass a narrow boundary;
3. one live unit changes;
4. settled units continue into a quiet history.

Motion should stop when the transition is complete. A permanently moving idle logo
would contradict the product's quiet, event-driven character. Animation must also have
a reduced-motion or static equivalent.

### 6.3 Color and contrast

A warm tea or amber accent can distinguish Oolong from cold terminal-tool palettes.
Neutral ink, charcoal, cream, and terminal-native foreground/background values should
carry most of the interface.

No meaning may depend on color alone. The wordmark and diagrams must work in monochrome,
and component themes must preserve readable contrast in real terminal color depths.
Exact palette values belong in a future visual asset specification, not in the
architecture or core packages.

## 7. The metaphor boundary

The brand may use flow and tea imagery. The public Go API uses domain language.

Prefer names such as:

- `Stream`, `Decoder`, `Transcript`, `Commit`, `Frame`, `Runtime`, and `Dispatcher`;
- `Editor`, `Dialog`, `List`, and other names for the behavior users actually need.

Do not introduce names such as `Brew`, `Steep`, `Cup`, `Leaf`, or `Infusion` merely to
match the product name. A metaphor that makes a user translate before understanding an
operation is an abstraction leak.

The same rule applies to the visual letter story. `oo`, `l`, `o`, and `ng` do not become
package names, protocol phases, or exported state constants. Brand meaning may explain
the system; it does not direct lower layers with upper-level vocabulary.

## 8. Standard copy

### Repository description

> A streaming-first terminal UI substrate for Go.

### Short introduction

> Oolong builds terminal interfaces around the lifetime of output. Open content stays
> live and interactive; finished content flows into terminal scrollback, keeping
> long-running sessions incremental and bounded.

### Component introduction

> Compose complete headless behavior with polished, themeable components, then keep
> product-specific workflow in the application.

### Supporting line

> Streams can be long. The live interface stays bounded.

These lines may be shortened for layout, but their meaning should not be replaced by a
generic claim such as "build beautiful TUIs". Appearance matters; the lifetime model is
what makes Oolong distinct.

## 9. Brand review checklist

Before publishing a README, website, release page, example, logo, or talk:

- Is the name spelled `Oolong` or `oolong`, never `ooloog`?
- Does the first technical description say streaming-first and terminal UI for Go?
- Does "streaming" mean incremental lifetime and publication, not merely animated
  text?
- Is boundedness presented as a tested architecture property rather than a mood?
- Are headless behavior, polished `kit` components, and application grammar distinct?
- Does the visual flow move left to right and settle when finished?
- Is the `n` legible, and does the mark work without color or motion?
- Are performance and compatibility claims backed by the repository's actual gates?
- Have tea and letter metaphors stayed out of package and API naming?
- Does the copy sound calm and exact rather than inflated or defensive?
