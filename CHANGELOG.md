# Changelog

## [0.2.1](https://github.com/swibrow/github-actions-tui/compare/v0.2.0...v0.2.1) (2026-03-04)


### Bug Fixes

* Checkout tagged commit in goreleaser step ([b6f6818](https://github.com/swibrow/github-actions-tui/commit/b6f6818214e6416349eb230bd17e7fc5c3fe30da))

## [0.2.0](https://github.com/swibrow/github-actions-tui/compare/v0.1.2...v0.2.0) (2026-03-04)


### Features

* Add 'o' key to open selected item in browser ([4afa2fa](https://github.com/swibrow/github-actions-tui/commit/4afa2fa2bc4eeae0deedb5165433da4dd6ffc265))
* Add 'p' key to open PR or branch in browser ([00be874](https://github.com/swibrow/github-actions-tui/commit/00be8742789641d88a682e32e84b2ea0429bb145))
* Add mouse support for all UI components ([43ddc65](https://github.com/swibrow/github-actions-tui/commit/43ddc65931fe2edd301120e1a37ca59191a41cb2))
* Click to drill in, right-click to go back, disable mouse in logs ([475cc20](https://github.com/swibrow/github-actions-tui/commit/475cc206158d41e004eaf72a8e8787e92b724be2))
* Migrate to charmbracelet v2 (bubbletea, bubbles, lipgloss) ([ca4f237](https://github.com/swibrow/github-actions-tui/commit/ca4f237781893451d132c75897a5a17ea2ff3389))


### Bug Fixes

* Allow feat commits to bump minor version pre-1.0 ([dbf7d91](https://github.com/swibrow/github-actions-tui/commit/dbf7d9139d42192338e704f76a0d664b763e497e))
* **deps:** update module github.com/google/go-github/v68 to v84 ([#15](https://github.com/swibrow/github-actions-tui/issues/15)) ([fdc1f30](https://github.com/swibrow/github-actions-tui/commit/fdc1f302228fa056ede1f33088a45ccfdd4eab2d))
* Honor scroll-to-step when tick resets loading flag ([1249903](https://github.com/swibrow/github-actions-tui/commit/12499037cd19e4b11193fcf6407239f57b19241b))
* Layout adjustments and navigation improvements ([ab00e67](https://github.com/swibrow/github-actions-tui/commit/ab00e6776776bba264dd14c2e615ecec804d259b))
* Reclaim wasted space in tree, graph, and table ([c91cf74](https://github.com/swibrow/github-actions-tui/commit/c91cf746efa5367237e32da0bd5136932adead45))
* Size table columns to content, grow only Branch and Actor ([2ae55d2](https://github.com/swibrow/github-actions-tui/commit/2ae55d298fd6e234ff70b0f556f5c3d1ed4c6672))

## [0.1.2](https://github.com/swibrow/github-actions-tui/compare/v0.1.1...v0.1.2) (2026-02-26)


### Features

* Add Homebrew tap support and --version flag ([e0fcd81](https://github.com/swibrow/github-actions-tui/commit/e0fcd81329aa4d32e8fdac677335b4e1e5632f0e))
* Add run attempt switching and SHA column ([505cd2b](https://github.com/swibrow/github-actions-tui/commit/505cd2bcdcfee698df93353bc75b24c6a9d0c204))


### Bug Fixes

* Resolve esc double-back navigation bug and fix lint issues ([25d8e50](https://github.com/swibrow/github-actions-tui/commit/25d8e50d4a6c9277656717f1ab086dfb989bea9b))

## [0.1.1](https://github.com/swibrow/github-actions-tui/compare/v0.1.0...v0.1.1) (2026-02-26)


### Features

* Add repo switcher, browser shortcut, adaptive columns, and release-please ([1655a99](https://github.com/swibrow/github-actions-tui/commit/1655a9947132f0f15e045146f2dc0d59bea931ec))
