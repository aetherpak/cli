# Changelog

## [0.16.0](https://github.com/aetherpak/cli/compare/v0.15.2...v0.16.0) (2026-06-07)


### Features

* auto-detect repository refs in push-oci ([dff23a0](https://github.com/aetherpak/cli/commit/dff23a0aa6bd7f4d4bc4d47baf3af56a80cd798e))

## [0.15.2](https://github.com/aetherpak/cli/compare/v0.15.1...v0.15.2) (2026-06-07)


### Bug Fixes

* **site:** prevent tag collisions in command syntax highlighter ([a34e812](https://github.com/aetherpak/cli/commit/a34e8126bb5f1cb15ec1ba02a718f9119f52148c))

## [0.15.1](https://github.com/aetherpak/cli/compare/v0.15.0...v0.15.1) (2026-06-04)


### Miscellaneous Chores

* release 0.15.1 ([093a0fe](https://github.com/aetherpak/cli/commit/093a0fe8ce8e8e95fb22c8da8d1820830136e6f1))

## [0.15.0](https://github.com/aetherpak/cli/compare/v0.14.2...v0.15.0) (2026-06-03)


### Features

* **builder:** support external remotes and build dependencies ([7cfb39d](https://github.com/aetherpak/cli/commit/7cfb39daf4f85bd683e5d0f72d4655668b3bbf02))

## [0.14.2](https://github.com/aetherpak/cli/compare/v0.14.1...v0.14.2) (2026-06-03)


### Bug Fixes

* handle signature backfill network/HTTP status errors as warnings ([862ef6a](https://github.com/aetherpak/cli/commit/862ef6a6292c0d3a3e6d2d2ab01416159df7692a))

## [0.14.1](https://github.com/aetherpak/cli/compare/v0.14.0...v0.14.1) (2026-06-03)


### Bug Fixes

* address correctness, reliability, and maintainability ([194b054](https://github.com/aetherpak/cli/commit/194b054c2b31f7421efdb7fc836cfb3be71142aa))
* validate signature backfill remote inputs and prevent path traversal ([f586a30](https://github.com/aetherpak/cli/commit/f586a30c21bd176bfd98451dd953d4d60afdb46d))

## [0.14.0](https://github.com/aetherpak/cli/compare/v0.13.0...v0.14.0) (2026-06-03)


### Features

* support multiple bundle URLs and combined path/url publication ([d6d6243](https://github.com/aetherpak/cli/commit/d6d6243c6e24273578fad307a15b3b887cf1a5ea))

## [0.13.0](https://github.com/aetherpak/cli/compare/v0.12.0...v0.13.0) (2026-06-03)


### Features

* add version command to CLI and inject version during build ([aa84497](https://github.com/aetherpak/cli/commit/aa84497a10416fb283931cf60ae30984f08558b3))

## [0.12.0](https://github.com/aetherpak/cli/compare/v0.11.2...v0.12.0) (2026-06-03)


### Features

* **linter:** support inline exceptions in config and CLI flag --linter-exception ([5745a9a](https://github.com/aetherpak/cli/commit/5745a9a1de61ad4f6330ebd85d67b81516827508))
* **linter:** support JSON exceptions configuration file and default ignores ([343d8af](https://github.com/aetherpak/cli/commit/343d8afd8bf995c51cfa2a442e210e19c531e29d))

## [0.11.2](https://github.com/aetherpak/cli/compare/v0.11.1...v0.11.2) (2026-06-02)


### Bug Fixes

* ignore Flathub screenshot-mirroring linter rules by default ([190482b](https://github.com/aetherpak/cli/commit/190482b3ee213e50a99bd390c01c00df3e68c1d9))

## [0.11.1](https://github.com/aetherpak/cli/compare/v0.11.0...v0.11.1) (2026-06-01)


### Bug Fixes

* **cmd:** allow publish with app-id and bundle-url env vars ([f57ba0b](https://github.com/aetherpak/cli/commit/f57ba0b0c2f718bdf9525dd82f9242a983e900d7))

## [0.11.0](https://github.com/aetherpak/cli/compare/v0.10.0...v0.11.0) (2026-06-01)


### Features

* **cli:** align command UX, configuration handling, error propagation, and testing ([8e51573](https://github.com/aetherpak/cli/commit/8e51573b397c3a8d24e321392981a313c38cb8ca))
* **cli:** support global output directory configuration for build assets ([e9b10af](https://github.com/aetherpak/cli/commit/e9b10afdf05d29fc07f230d44c8dc8aa2988798e))
* **cli:** support multi-app command execution, validate arch, and align oci_repository ([3fb7078](https://github.com/aetherpak/cli/commit/3fb70788332671c38f7c9a78751b37e0f73a88d3))
* **plan:** enforce manifest and force exclusion for CLI while allowing env vars ([15a6655](https://github.com/aetherpak/cli/commit/15a6655a899899ed4501ccd3fc12eef88b616acc))
* **publish:** support manifest and bundle one-off publishing with TTY confirm ([22cab9f](https://github.com/aetherpak/cli/commit/22cab9f0146227cbe4adfdd3553419b9396e0bbf))

## [0.10.0](https://github.com/aetherpak/cli/compare/v0.9.0...v0.10.0) (2026-06-01)


### Features

* **record:** support recursive cell record search ([d0f7b68](https://github.com/aetherpak/cli/commit/d0f7b68adf5a45e985412d7e3226310bc38b5486))

## [0.9.0](https://github.com/aetherpak/cli/compare/v0.8.0...v0.9.0) (2026-06-01)


### Features

* add 'aetherpak add' command to onboard apps into aetherpak.yaml ([6da0de4](https://github.com/aetherpak/cli/commit/6da0de4b3e4142f7d69bc53c199e92c7ef362c68))
* **cli:** add status diagnostics command ([09b2e24](https://github.com/aetherpak/cli/commit/09b2e248879ebc83d2f34d82018502c434d387fb))
* **cli:** support structured index templates and env vars for Pages URL and Index Template ([6fa0544](https://github.com/aetherpak/cli/commit/6fa05446bc87928d19cf07471b09aea4b40c3ebc))


### Bug Fixes

* **status:** check key decryption status and fix path fallback resolution ([92d00f3](https://github.com/aetherpak/cli/commit/92d00f36e3c1d5c8a0a487736cd2e8497a560145))

## [0.8.0](https://github.com/aetherpak/cli/compare/v0.7.0...v0.8.0) (2026-05-31)


### Features

* **ci:** add e2e container smoke test for amd64 and arm64 ([867ea23](https://github.com/aetherpak/cli/commit/867ea23f39be117551ae2e9b517ddbf405f62b2b))
* **ci:** cache go-build in container build action ([79ebefd](https://github.com/aetherpak/cli/commit/79ebefd2c2fe56dfbd6513b48be1fb002c27f762))
* **ci:** parallelise multi-platform container builds using workflow matrix ([b0461f3](https://github.com/aetherpak/cli/commit/b0461f36893f5f0c5e8762fe47e7c2931598c060))

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
