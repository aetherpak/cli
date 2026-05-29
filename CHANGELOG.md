# Changelog

## [0.6.0](https://github.com/aetherpak/cli/compare/v0.5.0...v0.6.0) (2026-05-29)


### Features

* **site:** clean up remaining title and tagline fallbacks in repo generator ([2e83ac9](https://github.com/aetherpak/cli/commit/2e83ac9d71e3664c5aa006cbd3332e2385f23f5c))
* **site:** rename 'repository registry' to 'repository' in meta description tags ([27fdda2](https://github.com/aetherpak/cli/commit/27fdda27ae00723c6acda5ac75a613460da2ec15))
* **site:** restore interactive card hover light glow effect ([ec913ee](https://github.com/aetherpak/cli/commit/ec913ee83a038cf5bece6cd5129e60c27c4f489d))

## [0.5.0](https://github.com/aetherpak/cli/compare/v0.4.0...v0.5.0) (2026-05-29)


### Features

* **site:** customize default landing page styling and headers ([232525a](https://github.com/aetherpak/cli/commit/232525a9ff7c1e776506130b163e1ee9243757e4))

## [0.4.0](https://github.com/aetherpak/cli/compare/v0.3.0...v0.4.0) (2026-05-29)


### Features

* **builder:** fail early on uninitialized git submodules ([b44b539](https://github.com/aetherpak/cli/commit/b44b539ef1c629a71af1287a2bc3638e5bb99c91))
* **config:** support builder_args in configuration ([7b71b2e](https://github.com/aetherpak/cli/commit/7b71b2eab2cf5f7d993bd96a3c5810c5957b23fd))
* **logger:** automatically enable plain output in CI environments ([46b0e3b](https://github.com/aetherpak/cli/commit/46b0e3b42be869f2f20c689e0160b70d9732d04f))
* **tui:** support scrolling log box in rich UI mode ([92b4846](https://github.com/aetherpak/cli/commit/92b4846fd9c25c1b36f7adc3c26bed59f9271707))


### Bug Fixes

* **tui:** resolve terminal resizing wonkiness in banners and logbox ([f3bfd17](https://github.com/aetherpak/cli/commit/f3bfd176673fe8b4c36c4a9956b1ee069c1ca1e0))

## [0.3.0](https://github.com/aetherpak/cli/compare/v0.2.2...v0.3.0) (2026-05-29)


### Features

* **builder:** pass extra flatpak-builder args and disable rofiles-fuse in CI ([875af7d](https://github.com/aetherpak/cli/commit/875af7d9efa0382f5a01f6adde544b1dae520044))

## [0.2.2](https://github.com/aetherpak/cli/compare/v0.2.1...v0.2.2) (2026-05-29)


### Bug Fixes

* trigger release ([7319d62](https://github.com/aetherpak/cli/commit/7319d624962ab81a6f76bdc027d7c236cb4ad641))

## [0.2.1](https://github.com/aetherpak/cli/compare/v0.2.0...v0.2.1) (2026-05-29)


### Bug Fixes

* **oci:** add required optional field to image signature payload ([8b91ffa](https://github.com/aetherpak/cli/commit/8b91ffafce7d7bc44347c03668c0fb44f305c7ce))

## [0.2.0](https://github.com/aetherpak/cli/compare/v0.1.0...v0.2.0) (2026-05-29)


### Features

* **cli:** release AetherPak CLI ([5df3af0](https://github.com/aetherpak/cli/commit/5df3af0e3476f84ed7043cd612db29a4064da54f))


### Bug Fixes

* **tests:** adapt integration test to flatpak version with out-of-band signature verification ([4727d65](https://github.com/aetherpak/cli/commit/4727d656d76d8bd88e18602bc1ec263f85b6d4d3))
* **tests:** pin push-oci to stable branch and format const block ([9066886](https://github.com/aetherpak/cli/commit/9066886383ca86fc7f8b80bc225ab088fa01d108))
