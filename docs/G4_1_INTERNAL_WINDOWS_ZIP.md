# G4.1 — внутренний автономный Windows x64 ZIP (без публикации)

**Статус:** G4.1 разрешён владельцем как feature-ветка и внутренний GitHub CI PoC. **Это не релиз, ZIP не загружается в GitHub artifacts** и не предоставляется пользователям. Исходная Agent main: `a396f251289ca4a135a2ef9056c3aafe95a7eca0` (A3.4 PR #7 объединён). Web остаётся только для чтения: схемы и каталоги не изменяются.

## Утверждённая архитектура

E+A: собственная программа Go + официальный закреплённый `llmfit` CLI как отдельный дочерний процесс. Пока `llmfit.exe` только включён в дистрибутив; UI не предлагает отдельный пользовательский запуск и **никогда не запускает его при открытии или быстрой диагностике**. Существующий `internal/llmfit/runner.go` остаётся адаптером без shell/свободных аргументов. Модели, веса и GPU/SYCL benchmark не загружаются и не выполняются.

## Точная структура внутреннего ZIP

```text
Neurometr-Windows-x64/
├── Neurometr.exe
├── bin/
│   └── llmfit.exe
├── bundled/
│   ├── catalog.json
│   └── catalog.lock.json
├── contracts/
│   ├── contract.lock.json
│   ├── ... все закреплённые схемы/fixtures/спецификации v0.1.0
├── LICENSES/
│   ├── llmfit-MIT.txt
│   ├── jsonschema-Apache-2.0.txt
│   ├── golang-x-text-BSD-3-Clause.txt
│   └── go-toolchain-BSD-3-Clause.txt
├── THIRD_PARTY_NOTICES_RU.txt
├── README_RU.txt
└── release-manifest.json
```

**Принципы:** `Neurometr.exe` использует `os.Executable()` для поиска соседних `contracts/` и `bundled/`, независимо от CWD. UI — `embed` HTML/CSS/JS внутри одного EXE. Данные истории — не в ZIP, а в личном `%LOCALAPPDATA%\Neurometr\history` с A3.4 проверкой Windows DACL.

`release-manifest.json` — **внутренняя инвентаризация**, не wire-contract Web и не подпись. Формат `neurometr.g4.1.internal-inventory`, статус `internal-ci-only-not-for-publication`, исходный Agent commit, диагностическая схема `0.1.0`, версия и SHA каталога, версия llmfit и SHA-256 **утверждённого официального ZIP**, SHA-256 извлечённого EXE, полный отсортированный перечень всех файлов (кроме самого manifest) с размером и SHA-256. Нет ключей, токенов, сетевых URL исполнения и автоматической загрузки.

## Закреплённый llmfit v1.1.20

- Источник: [AlexsJones/llmfit v1.1.20](https://github.com/AlexsJones/llmfit/releases/tag/v1.1.20), архив `llmfit-v1.1.20-x86_64-pc-windows-msvc.zip`.
- **6878103 байта**, **SHA-256 `1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489`**. Исходная проверка A2: [Windows CI #43061260905](https://github.com/Herurg123/neurometr-agent/actions/runs/43061260905).
- Во **временном Windows CI** скачивается только этот конкретный архив, сверяется длина, SHA, безопасный состав; извлечённый `llmfit.exe` проверяется `Get-AuthenticodeSignature` (`Valid`) и `--version` (строго `llmfit 1.1.20`). При несовпадении **сборка завершается отказом**, архив не используется.
- В ZIP копируется только подтверждённый бинарник из этого архива, а не неизвестная системная установка. `llmfit` не подключён к обязательной диагностике.

## Консистентность каталога и контрактов

- Contracts: `contract.VerifyPin` из зафиксированного Web commit `97aa0f4c1ceaef36608288e589b8abe935c5189e`; схема diagnostic `0.1.0`; копируются все canonical файлы и `contracts/contract.lock.json` без переписывания. FullValidator собирается из физически распакованного комплекта.
- Catalog: `bundled/catalog.json` версия `2026.11.11.2`, **6589 байт**, SHA-256 `1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2`. Сверяется через `catalogpin.Verify`, полный Go validator, lock и байтовый pin. Никаких network updates/Ed25519 remote manifest.

## CI и критерии готовности

**Стандартные Go тесты** на Ubuntu/Windows: существующие F01–F08, A1/A2/A3, защищённый localhost, история, pin, негативные тесты ZIP и `go vet`.

**Сборочный Windows job** после тестов:
1. Приватно получить официальный архив llmfit, проверить SHA/размер/структуру/Authenticode/`--version`; не выводить сырой stdout CLI.
2. `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o <TEMP>\Neurometr.exe ./cmd/localui`, Go 1.23.12. Убедиться в автономном Windows x64 PE.
3. `g4pack pack` создаёт ZIP только в `RUNNER_TEMP`; `g4pack verify` сверяет полную инвентаризацию и безопасно извлекает в **путь с пробелами**; повторная упаковка теми же входами даёт точный SHA-256 архива.
4. После распаковки в чистый путь без `go`/SDK в `PATH` запустить `Neurometr.exe -no-browser` **без `-root`**, с рабочей директорией вне пакета. Запрос к `127.0.0.1`, проверка Host/Origin/CSRF и запрет внешних интерфейсов. Native diagnostics, bundled `catalog_ref`, история между рестартами, ручной экспорт, Go FullValidator и независимый Web Python oracle.
5. Проверить, что при удалении `bin/llmfit.exe` или замене на повреждённый файл **не блокируется быстрая диагностика**. Не выдавать presence binary за running model/working backend.
6. Негативные тесты: неправильные размеры/хэши, path traversal, ZIP symlink, дубликаты имён с игнорированием регистра, несовпадение manifest, отсутствующий файл; без публикации приватных диагностических JSON/ZIP.

**Pass:** контрольная сумма официального архива совпадает; Authenticode `Valid`, версия `1.1.20`, упакованный комплект автономно работает без CWD/SDK, полный Go+Python JSON validator проходит, история после рестарта сохраняется, защита localhost не ухудшена, ZIP воспроизводим, повреждения блокируются.

## Лицензии и release blockers

В `packaging/licenses` лежат байтовые копии полных upstream LICENSE, независимо сверенные по tag/commit:
- llmfit `v1.1.20` — MIT, Copyright (c) 2026 Alex Jones.
- `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3` — Apache-2.0.
- `golang.org/x/text v0.14.0` — BSD-3-Clause.
- Go toolchain/runtime `go1.23.12` — BSD-3-Clause.

`THIRD_PARTY_NOTICES_RU.txt` перечисляет источники/версии и **открытые release blockers**: отсутствует утверждённая лицензия собственного Agent; не завершён аудит транзитивных Rust-зависимостей официального llmfit; необходима проверка прав на перераспространение всех комплектных материалов. **Лицензия Agent не назначается автоматически.**

## Что не тестируется

Нет пользовательского ZIP/GitHub release/deploy. Не выполняются G4.2 или тесты на ПК владельца. Не проверены фактический GPU/SYCL inference, NPU, TPS, модельные веса, сеть пользователя, SmartScreen на ПК владельца. История не зашифрована; стандартные loopback/ACL ограничения A3.4 сохраняются. Упаковка и ZIP manifest не являются подписью издателя.

## Контрольная точка внутренней сборки G4.1

[GitHub Actions #43243043079](https://github.com/Herurg123/neurometr-agent/actions/runs/43243043079), реализация packaging/smoke на кодовом HEAD `96685fe8eb3cbcc61e5d6708bea4a8cfcbc68d9d`: **3/3 PASS**.

- Ubuntu 24.04 и Windows 2025 x64: по **218 PASS, 0 FAIL, 2 ожидаемых SKIP** live-only; `go vet`, F01–F08, проверка canonical 23-файлового pin и bundled-каталога PASS.
- Официальный llmfit: `6 878 103` байта, SHA-256 `1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489`, `Authenticode=Valid`, `--version=llmfit 1.1.20`.
- Дважды собранный внутренний ZIP с **34 инвентаризованными файлами** оказался байтово одинаковым: **12 843 925 байт**, SHA-256 `7071f094f5d321fe3d38b832176b3794f4297a7447b3859a24784a7529676d79` для указанного source HEAD. Истинная длина/хэш ZIP зависит от исходного commit в release-manifest; повторная сборка на другом commit создаст другую контрольную сумму.
- Распакованный в путь с пробелами ZIP прошёл `contractcheck`, `catalogcheck`, проверку manifest и хешей, самостоятельный Windows localhost без `-root` и SDK в `PATH`; полный Go/Python validator для обычного native JSON, `catalog_ref.origin=bundled`, история после рестарта и ручной экспорт PASS. На runner создан диагностический JSON **2285 байт**, один DXGI-адаптер; это не данные ПК владельца.
- Шесть **негативных проверок настоящего полного ZIP**: отсутствие файла, подмена содержимого, подмена manifest, `../` traversal, дубликат пути с различием регистра и ZIP symlink — все отвергнуты **до записи на диск**.
- Быстрый маршрут продолжил работу при отсутствии и при повреждении `bin/llmfit.exe`; вызовы llmfit `system`, `fit`, `recommend` не выполнялись. `G4_1_PRIVATE_ZIP_CLEANUP=PASS NO_ARTIFACT_UPLOAD`: ни архив, ни приватный диагностический JSON не публиковались.

**Правовой release-blocker остаётся:** нет утверждённой лицензии собственного Agent; полный аудит транзитивных Rust-зависимостей и юридических условий распространения отложен до решения владельца. Внутренняя техническая проверка не снимает эти блокеры.

**Статус:** [Draft PR #8](https://github.com/Herurg123/neurometr-agent/pull/8). Окончательный HEAD ветки и финальный повторный CI после документов указываются в PR. Без отдельного решения владельца **merge, GitHub Release, ZIP artifacts, deployment и G4.2 запрещены**.

