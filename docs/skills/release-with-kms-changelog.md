---
title: Generate CHANGELOG.md via kms:changelog before every tag
status: active
date: 2026-08-08
tags: [paddle, provider, release, process]
---

Before pushing any release tag, run `kms:changelog` to update
`CHANGELOG.md` from commit history since the last tag, and commit it before
tagging.
