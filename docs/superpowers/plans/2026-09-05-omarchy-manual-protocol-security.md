# Omarchy Manual, Protocol Guide and Security Evidence — Implementation Plan

**Goal:** Подготовить проверяемый пакет документации для независимого A2UI CLI-прототипа и обсуждения его включения в Omarchy.

**Architecture:** Пользовательская глава описывает двусторонний сценарий «агент показывает варианты → пользователь выбирает → агент обновляет экран». Нормативная спецификация шести операций остаётся в `references/PROTOCOL.md`; отдельный guide связывает её с примерами, а security model — с кодом и проверками.

**Tech Stack:** Markdown, существующие JSON Schema/NDJSON fixtures и Go conformance tests, реальный Omarchy terminal для пользовательской проверки.

**Spec:** Основа — `references/PROTOCOL.md`, `assets/schema.json`, `references/OMARCHY.md`; целевой сценарий — самостоятельные `a2uid` + CLI для агента + терминальный `a2ui`.

## Scope and evidence baseline

- План составлен по локальной feature branch на `0426636d163e693712dbb3f80685a58f222a696f`, содержащей merge Epic 1 `540b8f6103d0c590c78a1e5e5f2ec93f7e7e7807`. Перед исполнением повторно зафиксировать HEAD; не считать локальный SHA автоматически текущим upstream.
- Это план документационного checkpoint 5. Он не реализует новый CLI, транспорт событий, упаковку, systemd или следующие Linux Epics.
- `references/OMARCHY.md` уже существует: переработать и связать его с новыми материалами, а не заводить вторую независимую спецификацию.
- Документация может готовиться параллельно CLI, но runnable quickstart и upstream submission зависят от готового двустороннего прототипа.
- Не выдавать наличие главы за требование DHH или гарантию принятия сопровождающими. Перед submission отдельно проверить действующие contribution rules выбранной версии Omarchy.

## 1. Positioning and editorial contract

Сохранить запрошенный pitch в proposal:

> A2UI: Zero-sandbox ephemeral UI engine for Omarchy agents.

Сразу дать техническое пояснение на английском:

> Agents describe interfaces with six bounded operations. A trusted local runtime renders them without executing generated UI code. The window is ephemeral; session state stays in memory while the daemon runs.

`Zero-sandbox` означает отсутствие необходимости исполнять сгенерированную UI-программу в отдельном sandbox. Это не утверждение об отсутствии привилегий, ненужности sandbox для агента или защищённости от любого локального процесса.

`Ephemeral` относится к окну и отсутствию гарантированной дисковой persistence. Различать закрытие клиента, остановку daemon и перезагрузку системы.

- [ ] Написать публичные материалы на английском, с короткими предложениями, быстрым первым результатом и командами непосредственно рядом с ожидаемым поведением.
- [ ] Проверить терминологию: шесть mutations, отдельные transport/session messages, отдельные пользовательские events; наличие `hello` не означает седьмую операцию.
- [ ] Согласовать рабочую формулировку сравнения: для задачи отображения UI A2UI уменьшает доступные действия по сравнению с выполнением произвольного сгенерированного Bash с правами пользователя.

## 2. Deliverables and ownership

| Файл | Назначение |
|---|---|
| `docs/manual/a2ui.md` (новый) | Короткая пользовательская глава; staging для upstream, точный upstream path выбирать по актуальному дереву Omarchy. |
| `references/OMARCHY.md` (существующий) | Обзор интеграции, pitch, ссылки на manual, protocol guide и security model. |
| `references/PROTOCOL.md` (существующий) | Единственный нормативный источник; исправлять только подтверждённые расхождения с реализацией отдельным review. |
| `docs/protocol-guide.md` (новый) | Практический walkthrough шести операций, sequencing, события и publication ACK. |
| `docs/security-model.md` (новый) | Threat model, сравнительная таблица возможностей и ограничения. |
| `docs/security-evidence.md` (новый) | Матрица claim → implementation → regression test → результат с SHA и toolchain. |
| `assets/examples/omarchy-choice.ndjson` (новый) | Одна воспроизводимая последовательность всех шести операций на существующих node IDs. |
| `docs/omarchy-submission.md` (новый) | Maintainer proposal: польза, воспроизведение, зависимости, доказательства, ограничения и стоимость сопровождения. |

Не дублировать полный справочник props в manual или skill. JSON-пример хранить в одном fixture; guide ссылается на него. README и `docs/agent-kit.md` получают ссылки на соответствующую аудитории точку входа.

## 3. Prerequisite: user-visible CLI round trip

**Consumes:** Реализованный CLI-прототип, его `--help`, способ установки и канал возврата событий.

**Produces:** Проверенный сценарий, пригодный для копирования в manual.

- [ ] Снять реальные команды из `--help` готовой версии. Ранее обсуждённые `a2ui-agent publish`, `wait-event`, `status` пока являются предложениями; не публиковать их как существующий API.
- [ ] Установить собранные binaries в тестовое окружение и выполнить quickstart из директории вне checkout. Не требовать Go или знания структуры worktree от обычного пользователя установленного приложения.
- [ ] Показать в отдельном окне таблицу двух вариантов и поле комментария. Скрипт-агент публикует UI, ждёт `select`/`submit`, получает значение и обновляет status. В этом демо не нужны LLM credentials или shell actions.
- [ ] Зафиксировать session ID, правила sequence ownership, ожидание event с таймаутом и поведение при reconnect. Проверить отсутствие потери/повторной обработки событий в пределах документированных гарантий CLI.
- [ ] Закрыть клиент до завершения сценария, открыть его заново, проверить сохранение ввода/выбора и завершить сценарий.
- [ ] Записать короткую демонстрацию на выбранной версии Omarchy. Подписать реальные процессы: agent script, daemon, terminal renderer. Не подменять её standalone `a2ui-runner` showcase.

**Acceptance:** Новый пользователь воспроизводит цикл без ручного JSON, переноса socket-переменных между shell и чтения исходников. Если CLI ещё не готов, документация явно имеет статус draft; screenshot/demo release gate остаётся открытым.

## 4. User chapter in the Manual style

**Files:** `docs/manual/a2ui.md`, `references/OMARCHY.md`, `README.md`.

- [ ] Структура главы: What it does → Install → Open a panel → Make a choice → Close and reopen → Stop the session → Troubleshooting.
- [ ] Начать с пользы и одного изображения настоящего UI. Дать минимальный runnable quickstart из шага 3 с описанием результата каждой команды.
- [ ] Описать Tab, навигацию таблицы, ввод и выход только по проверенному поведению текущей версии. Объяснить single interactive client и различие закрытия окна/daemon.
- [ ] Указать, что skill даёт агенту инструкции, а обмен операциями и событиями выполняет CLI. Не обещать автоматическое пробуждение Codex или встроенный UI внутри чата.
- [ ] В Troubleshooting оставить конкретные симптомы: daemon отсутствует, session не совпадает, client занят, timeout ожидания пользователя, отсутствует TTY.
- [ ] Дать короткую ссылку на security model вместо технической threat matrix внутри пользовательской главы.
- [ ] Не объявлять доступными `omarchy a2ui`, hotkey, package или menu item до реализации и принятия интеграции.

**Acceptance:** Глава читается самостоятельно; все команды существуют в выбранной сборке; developer test commands и детали IPC не мешают первому запуску.

## 5. Six-operation Protocol Guide

**Files:** `docs/protocol-guide.md`, `assets/examples/omarchy-choice.ndjson`; использовать существующие `wire/examples_test.go` и `conformance/` как основу проверок fixtures.

| Операция | Что обязательно объяснить | Проверка в общем walkthrough |
|---|---|---|
| `upsert` | Создание; full props replacement; существующий container parent; ограничения смены типа/parent. | Создать panel, text, selectable table, input и временный узел. |
| `props` | Shallow top-level merge; вложенный `style` заменяется целиком. | Обновить label/style без пересоздания дерева. |
| `text` | Append только в `text`/`viewport`; per-node и aggregate text limits. | Дописать строку в заранее созданный log. |
| `remove` | Удаление subtree; root неизменяем; очистка runtime state. | Удалить временный узел; отдельно проверить focus reconciliation. |
| `focus` | Существующий focusable target; runtime projection, не изменение Document revision. | Перевести focus в input. |
| `commit` | Publication barrier, не транзакция вокруг предыдущих операций; frame/through_seq/revision. | Завершить последовательность и дождаться соответствующего publication event. |

- [ ] Для каждой операции дать обязательные поля, defaults, результат, одну типичную ошибку и ссылку на нормативный раздел.
- [ ] Развести raw operation, hardened envelope и transport encoding. Показать handshake и последовательность с 1 без пропусков, duplicate handling, независимые event seq и Document revision.
- [ ] Создать один целостный fixture: все ID существуют к моменту обращения; parent создан до child; commit завершает сценарий. Проверить schema и replay reducer, поскольку schema не доказывает корректность последовательности.
- [ ] Добавить trace пользовательского ответа и terminal publication; объяснить, что mutation acceptance/HTTP 204 не является renderer ACK.
- [ ] Проверить реальное место отправки `frame_published` в `adapter/bubbletea/model.go` и границу Bubble Tea output. ACK описывать как подтверждение renderer path данной реализации. Не обещать физический показ пикселей или прочтение человеком без отдельного доказательства.

**Acceptance:** Все шесть операций покрыты исполняемыми примерами; guide не добавляет седьмую mutation и не расходится с `PROTOCOL.md`/schema.

## 6. Threat model and bounded safety claim

**Files:** `docs/security-model.md`, `docs/security-evidence.md`; скорректировать чрезмерные утверждения в `references/OMARCHY.md` на основе доказательств.

Определить assets: файлы и процессы пользователя, terminal/clipboard, input values, UI/session integrity, host actions, CPU/RAM. Trusted computing base: daemon, renderer, terminal emulator, зависимости и зарегистрированные host handlers. Agent-supplied content считать недоверенным.

Схема для документа:

```text
agent data → transport access policy → strict decoding/session validation
          → candidate validation/limits → Document + Runtime
          → snapshot → trusted renderer → terminal
user input → runtime event → event delivery → agent
explicit action → registered host handler → host policy / side effects
```

| Утверждение | Исходная опора | Что требуется доказать / ограничить |
|---|---|---|
| UI operations не являются shell execution API | `protocol/types.go`, `document/reducer.go`, `runtime/actions.go` | Проследить six-op path; отрицательный пример с shell-looking text и неизвестным action. Не выполнять настоящий сгенерированный Bash ради сравнения. |
| Невалидная mutation не меняет Document | `wire/json.go`, `protocol/validate.go`, document tests | Unknown fields, duplicate keys, invalid parent, invalid props; snapshot/revision до и после совпадают. |
| Retained resources ограничены | protocol limits, document invariant tests, `ipc/limits.go` | Различать agent record, retained Document, IPC snapshot; не называть это полным CPU/connection DoS control. |
| UDS доступен в пределах user account | `ipc/socket.go`, socket tests | Mode 0700/0600, startup lock, ownership/symlink/path checks отдельно. Same-UID процесс и root не изолированы этими modes. |
| HTTP имеет собственную access boundary | `daemon/mcp.go`, `cmd/a2uid/main.go` | Session ID и MCP headers не являются authentication. На просмотренной версии handler не показывает authentication; loopback не доказывает caller identity. Выбор production agent ingress — prerequisite security review. |
| Текст не исполняет terminal controls | `adapter/bubbletea/render_components.go`, `render_helpers.go` | Проверить raw ESC/CSI/OSC 52/OSC 8, C1, CR и fragmented text appends во всех text-bearing props. Отсутствие ANSI operation не является доказательством sanitization. Захватывать renderer bytes, не посылать probe в рабочий terminal пользователя. |
| Host action доступен только по разрешению | `runtime/actions.go`, action tests | Registration ограничивает vocabulary, но handler исполняется с host privileges. Timeout отменяет context, а не принудительно останавливает handler. Уточнить validation args/подтверждения по policy. |
| Commit нельзя подтвердить stale snapshot | daemon publication и IPC release tests | Exact generation, duplicates, monotonic snapshots, detach-before-ACK; отдельно граница доверия client ACK и отсутствие human-attention guarantee. |

- [ ] Разделить actors: недоверенная модель, другой Unix user, same-UID процесс, сетевой caller, скомпрометированный renderer/host handler.
- [ ] Сравнивать A2UI с выполнением произвольного generated Bash с теми же user privileges и без дополнительного confinement. Не обобщать на любой Bash или sandboxed application.
- [ ] Для каждого claim записать SHA, code path, test command, observed result и residual risk. Неподтверждённые строки маркировать `UNVERIFIED`, обнаруженные нарушения — `FAILED`.
- [ ] Security gaps оформить отдельными implementation задачами с regression proofs; prose не может заменить исправление. До их закрытия сузить публичную формулировку.
- [ ] Отдельно описать UI spoofing, утечку пользовательского ввода агенту, dependency vulnerabilities и отсутствие защиты от уже скомпрометированного user account.

**Acceptance:** Сравнение обосновывает меньшее число доступных действий при отображении UI. Нет обещаний абсолютной безопасности, sandbox для агента, безопасных произвольных actions или доказанного ANSI filtering без проверок.

## 7. Verification and maintainer submission

- [ ] Повторить quickstart в чистом пользовательском окружении выбранной версии Omarchy и записать версии terminal/Go/build SHA. Проверить install/uninstall instructions в разрешённом тестовом окружении.
- [ ] Прогнать schema/replay новых fixtures, targeted security cases и существующие gates: `make smoke-http`, `make smoke-ipc`, `make test-reattach`, `make test-startup-stress`, `make test`, `make test-race`, `go vet ./...`. Назначить fixture check в CI до submission.
- [ ] Проверить ссылки и корректность claims вручную; не писать тесты, привязанные только к формулировкам Markdown.
- [ ] Подготовить proposal: pitch, проблема, короткий demo, install/run path, compatibility matrix, dependencies, resource measurements, security evidence и план сопровождения.
- [ ] Указать, что theme, tiling, status bar, clipboard/Vim и service activation относятся к следующим checkpoints, если ещё не реализованы.
- [ ] Подготовить локальный reviewable diff; публикацию в upstream Omarchy выполнять отдельным согласованным действием.

Предлагаемые review units: (1) protocol guide + replay fixture; (2) security model/evidence и корректировки claims; (3) пользовательская глава + demo + submission draft. При обнаружении code gaps их исправления идут отдельно. Историю не переписывать.

## Sources and attribution

- [The Omarchy Manual: AI](https://omarchy.org/manual/ai/) — просмотрено 2026-09-05; ориентир для agent UX и ссылок на расширенные инструкции.
- [The Omarchy Manual: Omarchy CLI](https://omarchy.org/manual/omarchy-cli/) — просмотрено 2026-09-05; ориентир для короткого command-first изложения. Не является доказательством будущего принятия A2UI.
- `references/PROTOCOL.md`, `assets/schema.json`, `references/OMARCHY.md` и перечисленные code paths — локальные первичные источники контракта и текущей реализации.
- [Видео, предоставленное пользователем, 00:34:08](https://www.youtube.com/watch?v=2IDjteRQgMQ&t=2048s) — содержимое недоступно при проверке; перед цитированием сверить название, автора, transcript и контекст. Не приписывать DHH утверждение о безопасности A2UI на основании одного ID/таймкода.
- Текст про историю просмотров и условия YouTube — служебный текст платформы, в продуктовую документацию не включать.
