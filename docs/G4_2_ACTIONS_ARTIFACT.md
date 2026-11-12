# G4.2 — тестовый Windows x64 ZIP через GitHub Actions, только main

**Статус:** Draft PR #9; не объединён. Утверждено владельцем 12.11.2026:
допустимы тестовые Windows x64 бинарные комплекты через GitHub Actions
artifacts с официальным подписанным llmfit v1.1.20 при соблюдении сторонних
лицензий и документированном остаточном binary provenance риске.
Запрещены merge без отдельного согласия, GitHub Release, deployment,
коммерческая поставка и OSS-лицензирование собственного Agent.

## Три независимых решения

1. **LEGAL_COMPLIANCE=PASS** только после проверки в CI применимых Rust/npm
   лицензий, оригинальных copyright/NOTICE, составных SPDX AND, MPL-2.0
   §3.2 source по HTTPS/SHA, MIT/Apache перехода rmcp и полных текстов.
2. **BINARY_PROVENANCE=NOT_VERIFIED** всегда: реконструкция source Cargo
   closure не является независимым binary SBOM для подписанного llmfit.exe,
   не доказывает активные features и встроенные npm/JS ресурсы.
3. **OWNER_RISK_ACCEPTANCE=APPROVED**: владелец принял этот конкретный
   остаточный риск исключительно для тестовых GitHub Actions artifacts.
   Документ: packaging/G4_2_OWNER_RISK_ACCEPTANCE_RU.txt.

**PUBLIC_ARTIFACT_GATE=APPROVED** только на main после полного успешного CI
и юридического evidence PASS. Из PR и feature-веток upload запрещён всегда.

## Проверяемые исходники и лицензии

- Официальный llmfit v1.1.20 Windows x64 ZIP 6 878 103 байта, SHA-256
  1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489.
  CI проверяет actual ZIP size/SHA, Authenticode Valid и --version 1.1.20.
- Закреплённый llmfit commit bb5771394e88e352c266f4bf8bdb7a5295a998a8.
  Для Windows MSVC x64 CLI целевой Cargo normal/build default features
  closure = 215 source packages из 659 workspace Cargo.lock records.
  На предыдущем CI захвачено 366 оригинальных Rust LICENSE/NOTICE-like
  файлов, сохранены имена, лицензии/SPDX, содержимое и SHA исходника.
- Пять crates без стандартного LICENSE/NOTICE файла: clipboard-win@5.4.1
  (upstream BSL), rmcp@3.5.1 и rmcp-macros@3.5.1 (MIT+Apache),
  llmfit@1.1.20 и llmfit-core@1.1.20 (root MIT).
  Отсутствие файла в crate НЕ равно отсутствию лицензии.
- MPL-2.0: colored@3.1.1 и option-ext@0.2.0 соответствующие source .crate
  доступны бесплатно, точные HTTPS URL и Cargo.lock SHA опубликованы в
  packaging/THIRD_PARTY_NOTICES_RU.txt; CI их загружает и сверяет SHA.
  Полный MPL-текст и права получателей сохраняются.
- Составные лицензии ring (Apache-2.0 AND ISC), matchit (MIT AND BSD-3),
  unicode-ident ((MIT OR Apache-2.0) AND Unicode-3.0) проверяются особо.
  Пять npm production MIT пакетов react/react-dom/scheduler/js-tokens/
  loose-envify проверяются по source lockfile и отдельным copyright текстам.
  Источник списка — НЕ binary SBOM.
- Контракты diagnostic v0.1.0 и bundled-каталог 2026.11.11.2,
  SHA 1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2,
  остаются прежними. Веса моделей не включены.

## CI до разрешённого merge

Workflow .github/workflows/g4-1-private-zip.yml:

1. Go матрица Ubuntu/Windows: go test -v -count=1 ./..., go vet,
   contractcheck, catalogcheck и hostile ZIP unit tests.
2. На настоящем Windows 2025 runner: точный Agent source SHA, frozen
   read-only Web Python oracle, скачивание official pinned llmfit ZIP,
   проверка Authenticode/SHA/size/version.
3. Checkout pinned llmfit source: cargo tree/metadata, npm lockfile,
   проверка 215-entry inventory, упаковка оригинальных Rust notices;
   юридический аудит с 2 MPL .crate HTTPS + pinned source archive SHA.
4. Standalone Neurometr.exe без CGO; два byte-identical ZIP; g4pack verify,
   безопасная распаковка с пробелами, проверка manifest SHA и pin
   контрактов/каталога; localhost/native Windows smoke, история и экспорт,
   независимый Python oracle и шесть негативных ZIP-сценариев.
5. В частном RUNNER_TEMP подготовка ровно двух файлов:
   Neurometr-Windows-x64-<commit>.zip и SHA256SUMS.txt. Проверяется
   check_download_payload.py --public. release-manifest.json содержит SHA
   каждого файла, но не является цифровой подписью.
6. publication_gate.py повторно проверяет source/license evidence и
   owner risk document; только successful main push/main dispatch может
   перейти в approved. Никакие PR не загружают artifacts.
7. Только после успешного gate upload-artifact выдаёт один Actions artifact
   с двумя файлами, retention 14 дней. Actions Summary содержит commit,
   фактический ZIP SHA/размер, три статуса и настоящую ссылку либо BLOCKED.

## Скачивание после разрешённого merge

Откройте https://github.com/Herurg123/neurometr-agent/actions/workflows/g4-1-private-zip.yml
и выберите успешный main run с реальной ссылкой на artifact. Внешний ZIP
GitHub содержит внутренний application ZIP и SHA256SUMS.txt.
Проверить внутренний ZIP SHA-256 в PowerShell:
Get-FileHash -Algorithm SHA256 -LiteralPath '.\Neurometr-Windows-x64-<commit>.zip'
При несовпадении не запускать; затем распаковать ZIP приложения и запустить
Neurometr.exe без SDK. Не отключать SmartScreen/Defender.

## Не проверялось

Независимый Rust binary graph/attestation, состав встроенного JS,
пользовательские ПК, GPU/SYCL/NPU, реальные запуски весов/TPS и cloud.
Подпись upstream llmfit не равна подписи собственного Neurometr.exe;
локальная история в профиле пользователя не зашифрована.
E4 CPU-only нельзя сопоставлять с GPU/SYCL прогнозом.

После CI оркестратор получает точный HEAD, job/run ссылки, исходные SHA,
успешность юридического аудита и количество PR artifacts (ожидается 0).
Merge в main только по отдельному разрешению.
