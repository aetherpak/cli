# Changelog

## [0.7.0](https://github.com/aetherpak/cli/compare/v0.6.1...v0.7.0) (2026-05-30)


### Features

* add container images ([f97f5e0](https://github.com/aetherpak/cli/commit/f97f5e03e01dcf1a9dcb17da205d4ce9e50c888f))
* add global log-file flag and error log preservation ([8136c4b](https://github.com/aetherpak/cli/commit/8136c4b1fa7798c7877a904b4e514997265c0d18))
* extract and expose runtime-version from manifest ([feb0dab](https://github.com/aetherpak/cli/commit/feb0dab451e8519117964efc54d8cddd34f4640f))
* respect XDG_RUNTIME_DIR for temporary file creation ([b17962c](https://github.com/aetherpak/cli/commit/b17962ca45b41dbffedeb090ab8994fae640e41c))
* simplify Containerfile to inherit from flatpak and flatpak-builder base images ([4b8823b](https://github.com/aetherpak/cli/commit/4b8823b9f9bc250aa6a7f3c8f17fd00664218938))
* simplify signature configuration to no-sign flag and enforce GPG by default ([51eb7eb](https://github.com/aetherpak/cli/commit/51eb7eb8f359410d8a56301bc3ff6ab71db04973))
* support planning against a single Flatpak manifest directly ([6b15dee](https://github.com/aetherpak/cli/commit/6b15deed7319afa09ae58588e04db2b6e838cb5f))


### Bug Fixes

* add local nil-checks before dereferencing linter strict value ([a144316](https://github.com/aetherpak/cli/commit/a1443166adeb45b3a7c0cbb9a1a50c34016948dc))
* consolidate config loading to read config file once ([ca636fb](https://github.com/aetherpak/cli/commit/ca636fb38386f7d7e868ee3f967fc488dfcde244))
* enforce http timeout and max size limit on bundle download ([eeeed65](https://github.com/aetherpak/cli/commit/eeeed65415836de98afbf21b2195f0a557454e7e))
* escape logo_url in logoHTML image builder to prevent html injection ([4ff06c5](https://github.com/aetherpak/cli/commit/4ff06c59f348484bcc90029373982d3d4fdd57a2))
* omit empty --arch and --default-branch flags in builder ([6bba316](https://github.com/aetherpak/cli/commit/6bba316a7e95c67d217d3e98c26e226719b4b064))
* propagate plan marshal errors, default nil labels to empty object, and restrict log file permission to 0600 ([12d8f79](https://github.com/aetherpak/cli/commit/12d8f79332ea0caf97265f82430ebb34a4339d46))
* read global command flags via viper to enable environment variables ([23e942a](https://github.com/aetherpak/cli/commit/23e942a746f10448c6a7bc1ae79dafa57de2e805))
* sanitize metadata fields to prevent INI injection in flatpakref and flatpakrepo ([0818226](https://github.com/aetherpak/cli/commit/08182261c74148bd183d500da2a260143aa219be))
* specify registry in container build action push steps ([151ddb6](https://github.com/aetherpak/cli/commit/151ddb67bd146aa5f0905f305607477a5de27c83))
* validate records on read in IterRecords ([acfcf97](https://github.com/aetherpak/cli/commit/acfcf97dc97645c6a581b8094babb21c78bf0b88))

## [0.6.1](https://github.com/aetherpak/cli/compare/v0.6.0...v0.6.1) (2026-05-29)


### Bug Fixes

* **site:** use html/template for landing page and backfill signatures concurrently ([96d64c5](https://github.com/aetherpak/cli/commit/96d64c54911c9b32575871239a750142cc102d84))

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
