---
title: Troubleshoot by owner
description: Find the failed contract before adding retries or synchronization rules.
contentType: Troubleshooting
---

# Troubleshoot by owner

Language: English | [简体中文](../zh/guides/troubleshooting.md)

## Content is visible but not permanent

Check `StreamDocument.Current`: source EOF, preparation revision and result state differ. Call `End` when the source ends, and accept a preparation of that final snapshot. A preview is intentionally not publishable. Check the Transcript retention policy, its item error and the publication receipt. Partial results require explicit policy.

## A query sees newer state than the terminal

Observe the model, layout and written progress separately. A prepared layout can be waiting for writer admission, and an admitted frame can be waiting for physical writing. Use the relevant written sequence when a test needs terminal appearance. Do not force rendering from an observation callback.

## A target or selection became stale

A target names one attachment generation. Re-resolve intent with a locator after removal or virtual-window movement. A logical selection names content identity and revision; an edited or differently projected result may invalidate it. Do not reinterpret the old bytes as cells in a replacement document.

## Publication is refused

`ErrCapacity` retains the caller's content and can resume after writer progress. `ErrTooLarge` requires a deliberate size or representation decision. `errors.ErrUnsupported` may indicate a native painter or an atom wider than the target. `ErrClosed` preserves a known closing or transport cause. Blind retries after accepted partial transport can duplicate output.

## Work remains busy after cancellation

Cancellation requests cooperation. Check worker outcome and delivery state independently. If `Run` ignores context or blocks in external code, Oolong cannot honestly call that work completed. A closed scope still revokes applying a late result to a detached interface.
