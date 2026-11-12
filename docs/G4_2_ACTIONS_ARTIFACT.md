# G4.2 — скачиваемый Windows x64 ZIP из GitHub Actions (лицензионная блокировка)

**Статус:** ограниченный Draft PR в `neurometr-agent`. Изменения не объединены. Владелец разрешил потенциальную публичную доступность **только Actions artifact** (не GitHub Release); дополнительных репозиториев и серверов нет. Однако **автоматическая публикация остаётся заблокированной** до документальной проверки условий распространения.

## Утверждённый baseline и источники

- Agent `main` при начале этапа: `dbdd5984823be88eacc407046af12cd86e8d154f`, объединённый G4.1 PR #8.
- [G4.1 CI #43260151749](https://github.com/Herurg123/neurometr-agent/actions/runs/43260151749): 3/3 PASS на этом точном merge commit, внутренний ZIP **12 843 925 байт**, SHA-256 `df5b8351be89449d3e554c0cc6120d9ff741fcd4ce834f980c9a7033953527a1`. Контрольная сумма относится **только к указанному commit**, не к будущему ZIP.
- llmfit v1.1.20: официальный GitHub release `AlexsJones/llmfit`; ZIP Windows x64 ровно **6 878 103 байта**, SHA-256 `1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489`. Текущий CI проверяет Authenticode `Valid` и `llmfit 1.1.20`.
- Diagnostic v0.1.0, исходные canonical contracts и bundled-каталог `2026.11.11.2` с SHA-256 `1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2` **не менялись**.

## Маршрут скачиваемого artifact

Существующий `.github/workflows/g4-1-private-zip.yml` расширяется:

1. `push` только в `main`; `workflow_dispatch` можно вызывать, но публикация разрешена **только** при ref `refs/heads/main`; `pull_request` продолжает выполнять полный тест, **никогда не публикуя** ZIP.
2. После Go-матрицы Ubuntu/Windows выполняется Windows 2025 job: checkout **точного** исходного commit, pin Web-oracle, проверка официального llmfit (размер/SHA/подпись/версия), автономная Go-сборка, два детерминированных ZIP, `g4pack verify`, безопасная распаковка и полная проверка manifest / canonical pin / bundled-каталога.
3. Реальный Windows smoke на распакованном комплекте: localhost Host/Origin/CSRF, native CPU/RAM/GPU, Go FullValidator и независимый Python Web, история между запусками, экспорт, 6 негативных ZIP-сценариев, отсутствие llmfit как обязательной зависимости. Отчёты с приватными аппаратными данными не загружаются.
4. В частном `RUNNER_TEMP` формируется папка **ровно из двух файлов**: `Neurometr-Windows-x64-<sha>.zip` и `SHA256SUMS.txt`, строка `<sha256> *Neurometr-Windows-x64-<sha>.zip`. Отдельный `check_download_payload.py` проверяет имена, SHA, содержимое ZIP/manifest, запрет скрытых/служебных/секретных файлов и отсутствие лишнего материала. `SHA256SUMS.txt` относится к **внутреннему ZIP приложения**, не к внешнему контейнеру GitHub Actions.
5. `publication_gate.py` проверяет событие, ref, точный checkout и `packaging/public_artifact_gate.json`. Политика **сейчас `blocked`** по оставшемуся Rust/notices и невозможности независимо аттестовать точный состав подписанного llmfit EXE. Решения владельца о тестовом бинарнике Agent, атрибуции контрактов и каталога уже зафиксированы. При блокировке CI остаётся зелёным после успешных тестов, но `upload-artifact` не вызывается.
6. Если **в будущем** все лицензионные требования будут подтверждены через отдельное согласованное изменение policy, дополнительно проверяется пользовательская маркировка ZIP/notice. Нельзя распространять ZIP со старым `internal-ci-only-not-for-publication`. Только после этого разрешён `actions/upload-artifact` на `main`; action закреплён по SHA `ea165f8d65b6e75b540449e92b4886f43607fa02` (v4.6.2), **retention 14 дней**, `include-hidden-files=false`, `if-no-files-found=error`, имя `neurometr-windows-x64-<sha>`. Никакого GitHub Release.
7. В GitHub Actions Summary выводятся фактические commit SHA, ZIP-имя, **размер**, SHA-256, версия llmfit, статус smoke и либо ссылка на реально выгруженный artifact, либо однозначный статус `BLOCKED`. Папка staging и временные ZIP удаляются с runner.

**Fail-closed гарантии:** PR/feature/не-main dispatch никогда не публикуют архив, даже при гипотетически одобренной лицензии; несовпадение source SHA и manifest блокирует сборку. Любой сбой тестов отключает upload. Текущий `blocked` policy означает ноль опубликованных ZIP в этой ветке.

## Лицензии и незакрытые условия распространения

**Факт:** полные тексты `llmfit-MIT.txt`, `jsonschema-Apache-2.0.txt`, `golang-x-text-BSD-3-Clause.txt`, `go-toolchain-BSD-3-Clause.txt` уже входят в комплект G4.1. GitHub upstream `llmfit@v1.1.20` объявляет MIT для workspace. У upstream отсутствует очевидный комплект с полными транзитивными notices в дереве исходников (видны `LICENSE` и лицензия python-компонента).

**Блокер 1: Rust dependencies/notices.** Windows CI построил целевой граф **215 package/version**, включая normal и build edges, и зафиксировал SPDX и доступные имена notice. Полный перечень: `packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt`, проверяется на расхождение с Cargo при каждом CI. **Не хватает полных copyright/NOTICE/license texts для применимых crates, включая MPL-2.0 и составные лицензии.** Точный граф официального подписанного EXE независимо не аттестован.

**Подтверждено 2: права Agent.** Владелец разрешил тестовые бинарные комплекты через публичные Actions artifacts; решение записано в `packaging/DISTRIBUTION_TERMS_RU.txt`. Исходный код Agent остаётся без назначенной open-source лицензии; это не разрешает Release или коммерческую поставку.

**Подтверждено 3: контракты и каталог.** Агент включает проектную pin-копию схем и каталог метаданных двух Qwen GGUF по сохранённым commit/SHA; веса, модельные карточки и пользовательские данные не включены. Источники и Apache-2.0 Qwen отмечены в `DATA_SOURCE_ATTRIBUTION_RU.txt`. HTTPS/Ed25519 remote catalog остаётся отдельной задачей.

**Подтверждено 4: маркировка ZIP.** G4.2 manifest содержит `user-qa-distribution` и `neurometr.g4.2.test-inventory`, а `check_download_payload.py --public` прошёл на текущем Windows CI. Это не GitHub Release; инвентарный manifest с SHA-256 не является подписью издателя.

Политика в `packaging/public_artifact_gate.json` устанавливает `status=blocked`; в текущем PR можно закрывать доказательные лицензионные требования, но переключать на `approved` без подтверждения нельзя. **Нельзя делать публикацию вручную в обход этого gate**.

## Как пользователь скачает файл после лицензионного допуска

1. Открыть [Agent → Actions → G4.1/G4.2 Windows ZIP](https://github.com/Herurg123/neurometr-agent/actions/workflows/g4-1-private-zip.yml).
2. Выбрать успешный запуск на `main`. На странице Summary проверить `Source commit`, `ZIP size`, `SHA-256`, `Windows smoke=PASS`; если указано `BLOCKED`, ZIP скачивать нельзя.
3. В секции **Artifacts** скачать `neurometr-windows-x64-<commit>`. GitHub отдаёт **внешний ZIP-контейнер артефакта**: сначала распаковать его, затем найти **внутренний ZIP приложения** и `SHA256SUMS.txt`.
4. Убедиться, что имя внутреннего файла `Neurometr-Windows-x64-<commit>.zip` соответствует Summary. Проверить его SHA-256 стандартным PowerShell без установки SDK:
   ```powershell
   Get-FileHash -Algorithm SHA256 -LiteralPath '.\Neurometr-Windows-x64-<commit>.zip'
   ```
   Полученные 64 hex-символа (регистр не важен) должны точно совпасть с `SHA256SUMS.txt` и Summary. При расхождении **не запускать**.
5. Распаковать внутренний ZIP в личную папку с пробелами, запустить `Neurometr.exe` без прав администратора. Для отдельного QA-чата **строго одно действие за раз**; не выключать Defender/SmartScreen, не отправлять сырые диагностические JSON.

## Что не тестировалось

Нет конечной лицензионной приёмки и нет разрешения обходить текущую блокировку; публичного скачиваемого ZIP до выполнения условий не будет. Работа на реальном ПК владельца, SmartScreen/Defender, конкретные GPU/NPU, модели, TPS, cloud checks и права текущего пользователя Windows не подтверждены облачным runner. История локальна и не шифруется.

## Контроль результата PR

После CI указать commit HEAD, ссылки Go+Windows jobs, подтверждённый `NO_ARTIFACT_UPLOAD`, SHA/размер внутреннего ZIP, список diff, `main` и Draft PR. **Merge только по отдельному решению оркестратора.**


## G4.2 — обновление аудита после решения владельца от 12.11.2026 (Draft)

Владелец **разрешил тестовые бинарные комплекты через публичные GitHub Actions artifacts**, без публикации исходного кода, OSS-лицензии или GitHub Release. В пакет теперь входят `DISTRIBUTION_TERMS_RU.txt`, `DATA_SOURCE_ATTRIBUTION_RU.txt`, `LLMFIT_WINDOWS_CLI_AUDIT_RU.txt` и подтверждённые полные MIT-тексты для пяти npm production packages. Manifest тестовой сборки использует `user-qa-distribution`, а не прежнюю внутреннюю маркировку.

Восстановлена **точная целевая область**: `llmfit` (TUI/CLI) + `llmfit-core`, Windows `x86_64-pc-windows-msvc`, Rust normal/build edges и default features. Dev-dependencies и `llmfit-desktop` не учитываются как runtime CLI. В upstream release recipe **сначала** выполняются `npm ci` и `npm run build`, затем Rust build: JS/CSS могут встраиваться в подписанный EXE через `build.rs`. Нельзя считать 659 записей всего `Cargo.lock` списком лицензий CLI.

В CI предусмотрено независимое чтение **точного upstream commit**, `cargo tree --locked --target ... --edges normal,build --package llmfit`, `cargo metadata --locked --filter-platform ...` и npm lockfile. Скрипт печатает разрешённый перечень Rust компонентов с версиями, SPDX и доступными license/NOTICE filename; полный отчёт сохраняется только во временном каталоге CI. Это целевая **реконструкция исходников**, не доказательство идентичного кода в официальном подписанном EXE.

**Открытый блокер:** полный Rust transitive SBOM и обязательные copyright/NOTICE/полные лицензионные тексты ещё не подтверждены и не включены в комплект, upstream подписанный EXE не даёт собственной сборочной аттестации содержимого Rust и npm dist. Поэтому `public_artifact_gate.json` остаётся `blocked`. Простая подмена строк `approved` или фиктивных SHA не пройдёт gate: нужны актуальный проверенный Cargo-отчёт на Windows runner, совпадающий с закреплённым SBOM, а также полный реестр и тексты notices. Сам ZIP теперь проходит публичную проверку маркировки в PR без публикации из PR.

**Примечание к Windows smoke:** negative `manifest_metadata` меняет именно значение `user-qa-distribution` на недопустимое `public-release`; проверка manifest формата и распределения синхронизирована с G4.2. Шесть hostile-ZIP сценариев должны оставаться активными.


## Проверенное target-исследование на Windows CI

[CI #43273311334](https://github.com/Herurg123/neurometr-agent/actions/runs/43273311334): Rust Cargo normal/build target closure = **215** записей; npm production = **5**, источники pinned, pull-only. В составе присутствуют лицензии MPL-2.0, BSL-1.0, Zlib, ISC, BSD-3-Clause и составные `AND`/Unicode-3.0 наряду с MIT/Apache. Реальный signed EXE всё ещё не снабжён сопоставимым upstream build attestation. Список 215 записей сохраняется в `packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt` и в архиве, сверяется со свежим Cargo graph на Windows runner. Это не утверждение об исполнении Rust SDK пользователем.


**Упаковка исходных Rust notices:** в Windows CI по подтверждённому Cargo.lock извлекаются оригинальные LICENSE/NOTICE-like файлы из source crate archives и детерминированно объединяются с названиями/version/SPDX и SHA каждого исходного файла в `LICENSES/rust-cli-source-license-texts.txt`. Файл добавляется в ZIP и его SHA фиксируется в package manifest. Это сокращает объём недостающих лицензионных текстов, но не легализует недостающие crate notices автоматически: `clipboard-win`, `rmcp`, `rmcp-macros` не содержат найденных стандартных notice-файлов; у llmfit/core действует отдельный upstream MIT. `public_artifact_gate` остаётся `blocked`, пока не подтверждены все обязательства и точная связь с официальным EXE.


**Исправление Apache-2.0:** историческая upstream копия лицензии `jsonschema` обрывалась в конце Section 9. В G4.2 заменена полным стандартным текстом Apache Software Foundation (включая END OF TERMS и APPENDIX) из официального Apache-репозитория. ZIP smoke требует маркеры полной лицензии. Для Qwen 0.5B/1.5B добавлено copyright `Alibaba Cloud (2024)` из их LICENSE, веса моделей отсутствуют.


**Дополнение исходных лицензий:** upstream `clipboard-win` BSL-1.0 сохранён из закреплённого commit `ed27008...` (более поздний master 5.4.3, не tag исходного crate 5.4.1); `rmcp`/`rmcp-macros` используют общий upstream LICENSE тега `rmcp-v3.5.1`, который явно сохраняет MIT для ранее не перелицензированных contributions при переходе на Apache-2.0. Оба полных файла находятся в `LICENSES/` ZIP. Отличать наличие полного текста от полного coverage copyright и аттестации фактического бинарного EXE.
