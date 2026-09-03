# Changelog

## [0.14.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.13.0...v0.14.0) (2026-09-03)


### Features

* rework SQL console ([#90](https://github.com/descope-sample-apps/pgpeek/issues/90)) ([dd1d5c6](https://github.com/descope-sample-apps/pgpeek/commit/dd1d5c62afaeea19a95eba990c0425bbb22942e1))

## [0.13.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.12.0...v0.13.0) (2026-08-12)


### Features

* add query preview, exact counts, and gzip exports ([07f3627](https://github.com/descope-sample-apps/pgpeek/commit/07f3627069937d511414b34cc99d2047787f59a0))
* **web:** show elapsed time for running queries ([#83](https://github.com/descope-sample-apps/pgpeek/issues/83)) ([1231cc3](https://github.com/descope-sample-apps/pgpeek/commit/1231cc30f5b4cbe88cb70a602117eed6455f0b90))


### Bug Fixes

* **web:** bound SQL autocomplete metadata loading ([#84](https://github.com/descope-sample-apps/pgpeek/issues/84)) ([06b0ee8](https://github.com/descope-sample-apps/pgpeek/commit/06b0ee8315c4d94590b043c7d48aba6cba8ab339))

## [0.12.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.11.4...v0.12.0) (2026-08-11)


### Features

* **guard:** allow EXPLAIN ANALYZE on read-only statements ([#81](https://github.com/descope-sample-apps/pgpeek/issues/81)) ([d197c2b](https://github.com/descope-sample-apps/pgpeek/commit/d197c2ba0e2c0fa6275a7b04ae2f25c89e74a353))

## [0.11.4](https://github.com/descope-sample-apps/pgpeek/compare/v0.11.3...v0.11.4) (2026-08-10)


### Bug Fixes

* bust the client cache on release ([#78](https://github.com/descope-sample-apps/pgpeek/issues/78)) ([3e9473d](https://github.com/descope-sample-apps/pgpeek/commit/3e9473dc5d72f2fc0e2940afdf82045ecc07f2ea))
* preserve rows with oversized cells ([#79](https://github.com/descope-sample-apps/pgpeek/issues/79)) ([f650258](https://github.com/descope-sample-apps/pgpeek/commit/f65025819ccb7418230260ad7d2969696f42a4b3))

## [0.11.3](https://github.com/descope-sample-apps/pgpeek/compare/v0.11.2...v0.11.3) (2026-08-09)


### Bug Fixes

* raise table catalog byte limit and make it configurable ([#76](https://github.com/descope-sample-apps/pgpeek/issues/76)) ([52846da](https://github.com/descope-sample-apps/pgpeek/commit/52846da874e91f24e29264805b1313e522251fd8))

## [0.11.2](https://github.com/descope-sample-apps/pgpeek/compare/v0.11.1...v0.11.2) (2026-08-05)


### Bug Fixes

* polish empty and loading states ([#72](https://github.com/descope-sample-apps/pgpeek/issues/72)) ([e1619c6](https://github.com/descope-sample-apps/pgpeek/commit/e1619c6387634021bfd9458ff81d3f4e7c26570d))

## [0.11.1](https://github.com/descope-sample-apps/pgpeek/compare/v0.11.0...v0.11.1) (2026-08-02)


### Bug Fixes

* **web:** wrap SQL result cells ([#70](https://github.com/descope-sample-apps/pgpeek/issues/70)) ([cc40671](https://github.com/descope-sample-apps/pgpeek/commit/cc40671deb8e563ac073ce15eeecf0c7a6dc61f1))

## [0.11.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.10.0...v0.11.0) (2026-08-01)


### Features

* add About and database diagnostics ([#69](https://github.com/descope-sample-apps/pgpeek/issues/69)) ([c8e1db8](https://github.com/descope-sample-apps/pgpeek/commit/c8e1db80123764098c2ab9ec40bae3ff198810ec))
* **web:** group table partitions in sidebar ([#67](https://github.com/descope-sample-apps/pgpeek/issues/67)) ([ed32d2e](https://github.com/descope-sample-apps/pgpeek/commit/ed32d2ea5ef34b77930bbcb93bedf4e0136d1d58))


### Bug Fixes

* **web:** surface status feedback inline ([#66](https://github.com/descope-sample-apps/pgpeek/issues/66)) ([4ced47f](https://github.com/descope-sample-apps/pgpeek/commit/4ced47f5fd6cba175d69473efa4313d6541eac4d))

## [0.10.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.9.0...v0.10.0) (2026-07-30)


### Features

* **web:** make exact locations shareable ([#64](https://github.com/descope-sample-apps/pgpeek/issues/64)) ([c38f948](https://github.com/descope-sample-apps/pgpeek/commit/c38f948849754ac2bb64e9effbd12a1652ba6b92))

## [0.9.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.8.0...v0.9.0) (2026-07-30)


### Features

* **mcp:** expose pgpeek with optional Descope OAuth ([#61](https://github.com/descope-sample-apps/pgpeek/issues/61)) ([cfae660](https://github.com/descope-sample-apps/pgpeek/commit/cfae660ab916a413e3ce03a297f9d8222bea4659))

## [0.8.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.7.0...v0.8.0) (2026-07-05)


### Features

* support Cloudflare Access headers ([#57](https://github.com/descope-sample-apps/pgpeek/issues/57)) ([cda00e8](https://github.com/descope-sample-apps/pgpeek/commit/cda00e8f80480063759606d6a0a0bcae2eb0232b))


### Bug Fixes

* show SQL query errors inline ([#59](https://github.com/descope-sample-apps/pgpeek/issues/59)) ([10fda5f](https://github.com/descope-sample-apps/pgpeek/commit/10fda5f8ed4f8b32c42fc1200aa6eff7fe1412b2))

## [0.7.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.6.0...v0.7.0) (2026-07-01)


### Features

* **web:** improve large table readability ([#56](https://github.com/descope-sample-apps/pgpeek/issues/56)) ([7cc02f9](https://github.com/descope-sample-apps/pgpeek/commit/7cc02f9dd08915fbb34447f7013612d4d1a4abda))


### Bug Fixes

* **db:** cast pattern filters to text ([#54](https://github.com/descope-sample-apps/pgpeek/issues/54)) ([8540b8b](https://github.com/descope-sample-apps/pgpeek/commit/8540b8b67fd8e506d78bb7901dc85848f8541146))

## [0.6.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.5.0...v0.6.0) (2026-06-29)


### Features

* **web:** add SQL autocomplete ([#51](https://github.com/descope-sample-apps/pgpeek/issues/51)) ([553b78c](https://github.com/descope-sample-apps/pgpeek/commit/553b78cd7bf7e2d4914a6a1b338ea5a68bbfb346))

## [0.5.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.4.0...v0.5.0) (2026-06-28)


### Features

* support multiple databases ([#44](https://github.com/descope-sample-apps/pgpeek/issues/44)) ([17433e8](https://github.com/descope-sample-apps/pgpeek/commit/17433e8f0992d7025837ac5c93eec12c168343fb))


### Bug Fixes

* **web:** contain large schema overflow ([#47](https://github.com/descope-sample-apps/pgpeek/issues/47)) ([02a28a6](https://github.com/descope-sample-apps/pgpeek/commit/02a28a63b60701d1b895ff916534069523111ae1))
* **web:** remove CodeMirror 5 stylesheet ([#48](https://github.com/descope-sample-apps/pgpeek/issues/48)) ([e630a12](https://github.com/descope-sample-apps/pgpeek/commit/e630a1224bbc08324867f0bf5c2c6cc7558b3d26))

## [0.4.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.3.3...v0.4.0) (2026-06-24)


### Features

* **web:** upgrade CodeMirror 5 → 6 (vendored, Renovate-tracked) ([#34](https://github.com/descope-sample-apps/pgpeek/issues/34)) ([a5e7dcf](https://github.com/descope-sample-apps/pgpeek/commit/a5e7dcf2c952c6650843c730b076e29d976135b4))


### Bug Fixes

* **security:** harden guard, catalog errors, headers, and CDN assets ([#29](https://github.com/descope-sample-apps/pgpeek/issues/29)) ([0b71a66](https://github.com/descope-sample-apps/pgpeek/commit/0b71a66ece87d347c48800856f2c91694125ac9c))

## [0.3.3](https://github.com/descope-sample-apps/pgpeek/compare/v0.3.2...v0.3.3) (2026-06-24)


### Bug Fixes

* push images to descope-sample-apps org + rename go module to match ([#37](https://github.com/descope-sample-apps/pgpeek/issues/37)) ([d70f804](https://github.com/descope-sample-apps/pgpeek/commit/d70f804b7a7344cc1ffdfe64fc41f2cdd62ce893))

## [0.3.2](https://github.com/descope-sample-apps/pgpeek/compare/v0.3.1...v0.3.2) (2026-06-23)


### Bug Fixes

* **deps:** update module github.com/jackc/pgx/v5 to v5.10.0 ([#27](https://github.com/descope-sample-apps/pgpeek/issues/27)) ([7a2ba1e](https://github.com/descope-sample-apps/pgpeek/commit/7a2ba1ea193cb5c0d8e2c18daaa08e28c4da482a))

## [0.3.1](https://github.com/descope-sample-apps/pgpeek/compare/v0.3.0...v0.3.1) (2026-06-23)


### Bug Fixes

* **deps:** update module modernc.org/sqlite to v1.53.0 ([#28](https://github.com/descope-sample-apps/pgpeek/issues/28)) ([a44186b](https://github.com/descope-sample-apps/pgpeek/commit/a44186bac130949ddf39196285f90eec264d4142))

## [0.3.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.2.0...v0.3.0) (2026-06-23)


### Features

* **web:** switchable color themes + theme gallery on site ([#20](https://github.com/descope-sample-apps/pgpeek/issues/20)) ([ad7eed0](https://github.com/descope-sample-apps/pgpeek/commit/ad7eed0f318cba79f8a97bbbc4cc68dba58e90a0))


### Bug Fixes

* **ci:** trigger release.yml via release-please App token + bump checkout to v7 ([#24](https://github.com/descope-sample-apps/pgpeek/issues/24)) ([b320f81](https://github.com/descope-sample-apps/pgpeek/commit/b320f811f57edf04da5bc5592bc85abd93a71301))

## [0.2.0](https://github.com/descope-sample-apps/pgpeek/compare/v0.1.0...v0.2.0) (2026-06-23)


### Features

* configurable runtime, RDS IAM auth, TLS, and 100% test coverage ([a531296](https://github.com/descope-sample-apps/pgpeek/commit/a5312966e8a6489e478cc56205cf8f8028d8d6b0))
* data toolbar — global search, per-column filters, click-to-sort ([b66308a](https://github.com/descope-sample-apps/pgpeek/commit/b66308afbef858a6d396700893394533c6b7b9b0))
* **dev:** docker compose dev stack with seeded demo data ([7ef6921](https://github.com/descope-sample-apps/pgpeek/commit/7ef692115b7e81e3544c6fec9411295d86321e3d))
* foreign-key click-through + dark editor fixes ([e05c4b0](https://github.com/descope-sample-apps/pgpeek/commit/e05c4b0fdc2fc2bbdff02c5282c730ab021d6c9c))
* pgweb-style table browsing (sidebar + Data/Structure tabs) ([6bdaa52](https://github.com/descope-sample-apps/pgpeek/commit/6bdaa52f15de9b0c7f8ebfb8be11f2feeb738d5f))
* **web:** complete Preact+htm UI migration ([ceb7131](https://github.com/descope-sample-apps/pgpeek/commit/ceb713157ba3240ee9109dd5da6762de44130adc))


### Bug Fixes

* **deps:** update module github.com/jackc/pgx/v5 to v5.9.2 [security] ([#4](https://github.com/descope-sample-apps/pgpeek/issues/4)) ([67048bd](https://github.com/descope-sample-apps/pgpeek/commit/67048bd2126d75166f5bacdb53df61f43e98d6c4))
* **deps:** update module modernc.org/sqlite to v1.51.0 ([#6](https://github.com/descope-sample-apps/pgpeek/issues/6)) ([f816324](https://github.com/descope-sample-apps/pgpeek/commit/f8163249b0576973ca22004a2048846905376ec7))
* **web:** distinguish empty database from loading in sidebar ([ec38156](https://github.com/descope-sample-apps/pgpeek/commit/ec3815693391b1ba6d45370d434092f0b91662af))
