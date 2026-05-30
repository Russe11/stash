---
name: questions
description: Open questions awaiting a human answer (server fork)
updated: 2026-05-29
---

# Questions

Open questions that need a *human* answer (intent, product decisions, ambiguities the code can't resolve). Remove when answered (and apply the answer).

<!--
### YYYY-MM-DD — <short question>
<context + why it can't be answered from code>
-->

### 2026-05-29 — Are metadataIdentify / stash-box actually configured on the deployed fork?
The server exposes a full scraper/identify/stash-box surface (`scrapeSingleScene`, `metadataIdentify`, `submitStashBoxFingerprints`) and a macOS "scraping/identify" client feature is proposed — but whether the running fork has scrapers/stash-box endpoints configured (in `config.yml`) determines if a client identify feature has anything to call. Not determinable from source.

