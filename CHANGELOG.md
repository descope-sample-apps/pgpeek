# Changelog

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
