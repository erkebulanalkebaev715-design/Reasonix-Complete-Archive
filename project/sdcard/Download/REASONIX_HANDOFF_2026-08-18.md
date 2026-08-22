# REASONIX / BALANCE MOD / ANDROID APK — HANDOFF
Дата фиксации: 2026-08-18

> Этот файл предназначен для нового чата ChatGPT.  
> НЕ заставляй пользователя заново объяснять проект. Считай этот документ основным контекстом и продолжай с текущего состояния.
>
> ВАЖНО: не выдумывай, что что-то протестировано, если ниже это помечено как НЕ ПРОВЕРЕНО.  
> Не проси API-ключ в чат и не печатай его. Все секреты остаются локально на телефоне.

---

# 1. Что вообще строим

Основной проект: **Reasonix Balance Mod** + мобильный Android APK-клиент.

Цель:

**MAX BALANCE = POWER / COST**

То есть максимальный реальный результат на:
- токен,
- тенге,
- секунду,
- память,
- попытку.

Архитектурный принцип:
- не создавать второй агентный движок;
- использовать существующий Reasonix:
  - controller;
  - agent;
  - tools;
  - sessions;
  - queue;
  - environment;
  - checkpoints/recovery;
  - serve/backend;
- APK должен быть тонкой мобильной оболочкой над тем же Reasonix/Balance Mod.

---

# 2. Устройство и среда пользователя

Основное устройство:
- Android;
- Realme Note 60;
- ARM64;
- Termux;
- `proot-distro`;
- Debian внутри PRoot;
- ПК нет.

Reasonix находится внутри Debian:

```bash
/root/DeepSeek-Reasonix
```

или:

```bash
~/DeepSeek-Reasonix
```

Рабочий Go:

```bash
/usr/local/go/bin/go
```

Для Go/Reasonix команд использовать:

```bash
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local
```

Последняя известная версия:

```text
go version go1.26.6 linux/arm64
```

---

# 3. Исходный Reasonix

Репозиторий:
- `esengine/DeepSeek-Reasonix`
- ветка: `main-v2`

Исходный revision проекта пользователя:

```text
9e68643823943f05d13ab6a4578b7a629d490b07
```

Ранее бинарник:

```bash
~/DeepSeek-Reasonix/bin/reasonix
```

---

# 4. Balance Mod — уже пройденные версии

На телефоне пользователя были успешно пройдены:

- v0.1 — foundation
- v0.1.1 — MockProvider capability proxy hotfix
- v0.2 — anti-loop/native completion gate/APK quality telemetry
- v0.3 — Power Router / Failure Cache / Log Reducer / Patch Governor
- v0.4 / 0.4.1 — JSON Content-Type hotfix
- v0.5 — Execution Router / model slots / Flash-Pro switching / budget recheck
- v0.6 / 0.6.1 — tool visibility fix
- v0.7 — APK protocol / Capability Registry / native environment / project profiles / Tool Packs / Chat-Agent / Live Project Protocol
- v0.8 / 0.8.1 — patch/rebase
- v0.8.2 — Persistence compile hotfix
- v0.9 — Project + Task Manager
- v0.10 — durable queue / recovery / per-task budget
- v0.11 — unified Power/Economy Engine
- v0.12 — Auto Continuation Orchestrator
- v0.13 — durable safety gate / Pro diagnosis read-only overlay / durable pending route / idempotent continuation
- v0.14.3 — FULL PASS
- v0.15 — OFFLINE STRESS GATE FULL PASS
- v0.16 — Hard Pre-Call Budget System FULL PASS offline
- v0.17 — crash / exactly-once hardening PASS
- v0.18 — Full Offline Prototype / RC FULL PASS
- v0.19 — APK Backend Integration FULL PASS
- v0.20 — First Real DeepSeek API Gate FULL PASS on Android ARM64

Не заявлять «универсальное exactly-once»: v0.15/v0.17 использовали более аккуратную семантику uncertain/receipt/ack.

---

# 5. Самый важный факт: v0.20 REAL API FULL PASS

Финальный доказанный реальный прогон:

```text
provider completion observed in ~3s | spentKzt=0.318653005 | liveText=masked | marker=masked-or-omitted
[REAL 13/14] verify Flash-only path + reconcile provider-reported usage
V020_RECONCILE_PASS prompt=5162 hit=256 miss=4906 output=11 requests=1 rateCardUSD=0.00069064 rateCardKZT=0.318653 backendDeltaKZT=0.318653 usageReceipt=exact
[REAL 14/14] final hard-budget assertion
spent=0.318653 KZT | remaining=9.681347 KZT | cap=10.00 KZT
BALANCE_MOD_V20_REAL_GATE_PASS
BALANCE_MOD_V20_SMOKE_PASS
```

Корректное утверждение:

> **v0.20 First Real DeepSeek API Gate FULL PASS on Android ARM64.**

Оговорка:
reconciliation = provider-reported usage × rate card против Reasonix ledger, а не независимый аудит списания кошелька DeepSeek.

---

# 6. Fixed12

Последний успешный bundle:

```text
reasonix-balance-mod-v0.20-fixed12-bundle.tar.gz
```

SHA-256:

```text
a8edf623b7b8d62dcb2eea1142c1c5a99320668b5fcd648e453af04a3dccc381
```

Fixed12:
- поддержал legacy и current формы `agent.emitTurnUsage`;
- структурно вставил exact usage receipt hook;
- не полагался на хрупкий текстовый anchor;
- после него реальный v0.20 gate прошёл FULL PASS.

---

# 7. История ключевых багов v0.20

## Fixed4
Использовал слишком консервативный reserve.

Старая логика:

```text
strictPreCallRetryFactor = maxSamplingAttempts * (provider.MaxRetries + 1)
```

Получалось:

```text
6 * (10 + 1) = 66
```

Реальный вызов блокировался ДО paid call:

```text
estimated input 0.011052 >= retry-reserved share 0.000328 $
```

## Fixed5
Идея:
- strict one paid provider attempt per admission;
- retries = 0 в strict hard-budget;
- replay отключён в strict режиме.

## Fixed6
Проектировался non-stream strict DeepSeek request + deadline.
Пользователь фактически Fixed6 не прогонял.

## Fixed7
Installer создал дубликат Go test:

```text
TestBalanceStrictRetryLimitZeroStartsOneHTTPAttempt redeclared
```

Rollback PASS.

## Fixed8
Добавили cleanup старых generated tests.
Targeted tests прошли, но широкий:

```bash
go test ./internal/agent
```

запустил environment-sensitive session/write-authority тесты под PRoot.

Были WARN:

```text
session: save without a held lease
```

и проблемы replay.
Rollback PASS.

Вывод:
НЕ запускать бездумно огромный `go test ./internal/agent` под PRoot как обязательный acceptance gate.

## Fixed9
Перешли на compile-only для touched packages:

```bash
go test ./internal/provider ./internal/agent ./internal/serve -run '^$' -count=1
```

Fixed9 локально PASS.

Первый реальный вызов модели реально завершился, spent:

```text
0.334987596 KZT
```

но harness ждал 90 сек из-за masked live text / literal marker assumption.

## Fixed10
Completion criterion поправили:
- positive spend;
- nonempty `live.chat.message`;
- clean `live.turn.done`;
- marker optional.

Реальный вызов:

```text
clean provider completion observed in ~3s | spentKzt=0.3187176
```

Но reconcile упал:

```text
V020_RECONCILE_FAIL: provider-reported usage not found
```

## Fixed11
Попытка usage receipt hook.
Installer ожидал current upstream `emitTurnUsage` форму и упал:

```text
ERROR: exact emitTurnUsage sink/return anchor count=0, expected 1
```

Причина:
исходный revision пользователя использует legacy void `emitTurnUsage`.

## Fixed12
Структурный patcher поддержал обе формы.
Финальный REAL FULL PASS.

---

# 8. Hard Budget — важное ограничение

Доказанный жёсткий бюджет был протестирован через Balance backend / `reasonix serve` / `/mod/budget`.

Обычный интерактивный:

```bash
./bin/reasonix
```

НЕ надо автоматически считать доказанно эквивалентным тому же hard-budget пути.

Если пользователь хочет реальную гарантию денежного cap:
- использовать Balance backend path;
- не обещать, что plain CLI сам даёт тот же tested hard cap.

---

# 9. DeepSeek provider — текущая настройка

В Reasonix был создан OpenAI-compatible provider:

```text
name: custom-api-deepseek-com
kind: openai
base_url_host: api.deepseek.com
models:
  deepseek-v4-flash
api_key_env: CUSTOM_API_DEEPSEEK_COM_API_KEY
is_default: true
```

Ранее `doctor --json` показывал:

```text
context_window: 128000
```

Потом context настраивали на:

```text
1000000
```

В PRoot sandbox Reasonix показывал:

```text
bash: enforce
available: false
```

Поэтому проектную настройку переключали:

```toml
[sandbox]
bash = "off"
```

Reasonix permissions/controller при этом остаются; отключается именно недоступный OS-level bash sandbox.

---

# 10. Auth / 401 — что произошло

При первом боевом CLI:

```text
Authentication failed (HTTP 401)
```

Причина была локально доказана командой на:

```text
CUSTOM_API_DEEPSEEK_COM_API_KEY
```

и `/models`.

Получили:

```text
DeepSeek auth HTTP=401
```

То есть это был не баг model ref/Reasonix — DeepSeek отклонил credential, сохранённый в новом provider slot.

Ранее реальный v0.20 gate успешно использовал:

```text
DEEPSEEK_API_KEY
```

Дальше credential синхронизировали локально.

ВАЖНО:
- пользователь не хочет, чтобы ему постоянно повторяли лекцию про ключ;
- НИКОГДА не печатать его значение;
- в handoff файл секреты не включены.

---

# 11. Кликер — тест Reasonix как coding agent

Тестовый проект:

```bash
/root/clicker
```

Reasonix реально создал:

```bash
/root/clicker/index.html
```

При записи Reasonix попросил permission, потому что `/root/clicker` был вне рабочего workspace.

Исправление:
Reasonix запускать прямо с:

```bash
--dir /root/clicker
```

или разрешить directory на session.

Кликер был создан и файл существовал.
Потом Reasonix ушёл в долгую auto-verification:

```text
This turn still owes chunks; Reasonix is finishing them automatically
```

Пользователь остановил/мог остановить `Ctrl+C`, файл от этого не исчезает.

---

# 12. Что теперь такое APK

ВАЖНО:
APK — НЕ кликер.

APK = мобильный UI / control center для Reasonix + Balance Mod.

Главное ТЗ APK 1.0:

1. Обычный чат с DeepSeek/ИИ.
2. История и несколько диалогов.
3. Выбор модели.
4. Возможность добавлять другие API/providers/models:
   - DeepSeek;
   - OpenAI-compatible;
   - Anthropic-compatible;
   - custom endpoints.
5. System prompts / global instructions / project instructions / chat instructions.
6. Budget:
   - текущая стоимость;
   - остаток;
   - лимит;
   - увеличение/уменьшение;
   - hard stop.
7. Activity:
   - tool calls;
   - команды;
   - stdout/stderr;
   - какие файлы прочитаны;
   - какие файлы изменены;
   - diffs;
   - тесты;
   - ошибки;
   - status.
8. НЕ обещать скрытый private chain-of-thought.
   Показывать:
   - provider-visible reasoning summary/fields, если доступны;
   - action trace;
   - tool activity;
   - краткие reasoning summaries.
9. Раунды:
   - analyze;
   - варианты;
   - выбор;
   - implementation;
   - verification;
   - fix.
10. Project manager.
11. Checkpoints/rollback.
12. Permission Center:
   - allow once;
   - session;
   - always;
   - deny.
13. STOP.
14. Task Queue.
15. Parallel agents/subagents.
16. Terminal.
17. File manager.
18. Logs/tests.
19. Context/token/cache display.
20. Cost estimation.
21. Profiles:
   - Economy;
   - Balance;
   - Maximum.
22. Templates.
23. Android notifications.
24. Export/import config.
25. Health dashboard.
26. Completion report.
27. Continue until result.
28. Attachments/files/images/projects.
29. No duplicated agent engine inside APK.

---

# 13. Tools & Skills — обязательно

APK должен иметь отдельный раздел:

```text
Tools & Skills
```

## Tools
Можно подключать:
- terminal;
- files;
- network;
- HTTP/API;
- Git;
- Android/ADB;
- custom tools;
- MCP servers.

Права tools:
- once;
- session;
- always;
- deny.

В Activity видно:
- имя tool;
- параметры;
- результат;
- время;
- error/success.

## Skills

Reasonix реально поддерживает Skills как Markdown playbooks.

Project skill path:

```text
.reasonix/skills/<name>/SKILL.md
```

Skill может быть:
- inline;
- subagent;
- manual invocation;
- tool allowlist;
- read-only;
- model override;
- effort override.

Пример:

```yaml
---
name: reviewer
description: Review changes for correctness and regressions
invocation: manual
runAs: subagent
model: deepseek-pro
effort: high
read-only: true
allowed-tools: [read_file, grep, bash]
---
You are a focused code reviewer...
```

Причём `allowed-tools` НЕ обходит permissions.

## MCP

Reasonix поддерживает `.mcp.json`.

Формат:

```json
{
  "mcpServers": {
    "name": {
      "command": "...",
      "args": [],
      "env": {}
    }
  }
}
```

Также remote:
- SSE;
- streamable HTTP.

---

# 14. Project Loadout

Запланированный UX:

```text
Model
+ System Prompt
+ Skills
+ Tools
+ Permissions
+ Budget
```

= один Loadout проекта.

Например:

```text
Android Dev
DeepSeek Flash
Android Developer Skill
terminal + files + Gradle + Git
project-only writes
20 KZT
```

---

# 15. UI APK

Пользователь хочет интерфейс максимально похожий по удобству на **мобильное приложение DeepSeek**:
- chat-first;
- чистый;
- не перегруженный;
- тёмный;
- современный.

Но это не должна быть тупая копия брендинга/логотипов.
Собственные функции Reasonix/Balance должны органично встраиваться.

Основные вкладки:

```text
Chat | Activity | Project | Models | Settings
```

Постоянный status header:

```text
MODEL · STATUS · CONTEXT · COST · BUDGET · STOP
```

---

# 16. Уже сгенерированные UI assets/textures

Была создана полная очередь визуальных assets:

1. главный dark background;
2. panels/cards;
3. chat bubbles;
4. buttons;
5. input controls/model pills/toggles;
6. icon pack;
7. status/budget/context/loading indicators;
8. dialogs/permissions/bottom sheets;
9. navigation;
10. activity/console/diff/log components.

Дополнительная полировка, которую можно сделать позже:
- app icon;
- splash screen;
- Reasonix Mobile mark;
- provider/model avatars.

---

# 17. APK v1.0 — созданные артефакты

В предыдущем чате были сгенерированы:

```text
Reasonix-Mobile-v1.0.apk
reasonix-mobile-v1.0-backend.tar.gz
reasonix-mobile-v1.0-source.tar.gz
reasonix-mobile-v1.0-source-private.tar.gz
Reasonix-Mobile-v1.0-SPEC.md
Reasonix-Mobile-v1.0-BUILD-REPORT.txt
Reasonix-Mobile-v1.0-ARTIFACTS-SHA256.txt
```

ВАЖНО:
не считать user-device acceptance завершённым.
В чате подтверждён запуск backend.
Установка/полноценный UI test APK на Android в этой точке ещё не подтверждены.

---

# 18. Backend APK — текущий реальный статус

Архив backend был распакован с лишним уровнем директории.

Фактическая папка:

```bash
/root/reasonix-mobile-v1.0-backend/reasonix-mobile-v1.0-backend
```

В ней пользователь подтвердил:

```text
START-HERE.txt
reasonix_mobile_backend.sh
reasonix_mobile_bridge.py
```

Запуск:

```bash
cd ~/reasonix-mobile-v1.0-backend/reasonix-mobile-v1.0-backend

bash ./reasonix_mobile_backend.sh start
bash ./reasonix_mobile_backend.sh status
bash ./reasonix_mobile_backend.sh token
```

Фактический подтверждённый status:

```text
status=running
bridge_pid=15344
url=http://127.0.0.1:37914
model=custom-api-deepseek-com/deepseek-v4-flash
reasonix_url=http://127.0.0.1:37333
token_file=/root/.reasonix-mobile-v1/serve.token
```

Пользователь также успешно получил token.

НЕ вставлять его в новый чат.
Получать заново командой:

```bash
bash ./reasonix_mobile_backend.sh token
```

---

# 19. Последняя точка перед переходом в новый чат

Backend УЖЕ РАБОТАЛ.

Следующий план был:

1. Установить:

```text
Reasonix-Mobile-v1.0.apk
```

2. Открыть APK.
3. Settings.
4. Backend URL:

```text
http://127.0.0.1:37914
```

5. Token:
получить локально:

```bash
bash ./reasonix_mobile_backend.sh token
```

6. Connect.
7. Chat test:

```text
Ответь одним словом: РАБОТАЕТ
```

Если ошибка:
- пользователь должен прислать скрин;
- чинить конкретный real-device bug;
- делать v1.0.1;
- не строить догадки.

---

# 20. Ошибки пользователя при командах — учитывать

Пользователь часто случайно склеивает команды или опечатывается.

Примеры:
- написал `d` вместо `cd`;
- `token~`;
- склеил:
  `tokencd ...`
- несколько команд вставлялись без нужного разделения.

Поэтому новому чату:
- давать короткие copy-paste блоки;
- желательно одна операция за раз;
- не использовать placeholder вроде `/путь/к/...` без предупреждения;
- если путь известен — писать точный путь;
- если неизвестен — сначала `pwd`, `ls`, `find`, потом точная команда.

---

# 21. Что НЕ делать новому чату

НЕ:
- заставлять заново ставить Reasonix;
- начинать проект с нуля;
- переубеждать в уже подтверждённых PASS;
- называть plain CLI доказанным hard-budget path;
- повторно просить «одобрение» для уже пройденного v0.20 gate;
- печатать или просить API key в чат;
- выдумывать DeepSeek provider/model refs;
- давать огромные ручные sed-патчи без необходимости;
- запускать broad environment-sensitive Go tests как обязательный gate;
- заявлять, что APK прошёл Android acceptance, пока пользователь реально не проверит;
- путать APK с тестовым clicker.

---

# 22. Стиль работы с пользователем

Пользователь предпочитает:
- русский;
- коротко;
- прямо;
- технически;
- без лести;
- без «розовых очков»;
- без повторных вопросов, если ответ уже известен;
- рабочие команды, а не теорию;
- если нужен fix — лучше полный replacement script/bundle, чем десяток ручных patch-команд.

Можно спокойно использовать грубый разговорный стиль пользователя, но ответ должен оставаться понятным.

---

# 23. Первое сообщение нового чата — рекомендуется

Пользователь может загрузить этот файл и написать:

```text
Прочитай этот HANDOFF полностью и продолжай проект строго с текущей точки.
Не начинай ничего заново.
Сначала коротко подтверди текущий статус:
1) Reasonix/Balance v0.20 Fixed12 real FULL PASS,
2) mobile backend уже был поднят на 127.0.0.1:37914,
3) следующий шаг — real-device test Reasonix-Mobile-v1.0.apk.
После подтверждения жди мой скрин/ошибку APK и чини только фактическую проблему.
```

---

# 24. Быстрый recovery backend после нового чата

Если Debian был закрыт/перезапущен:

```bash
cd ~/reasonix-mobile-v1.0-backend/reasonix-mobile-v1.0-backend

bash ./reasonix_mobile_backend.sh start
bash ./reasonix_mobile_backend.sh status
```

Получить token:

```bash
bash ./reasonix_mobile_backend.sh token
```

Логи:

```bash
bash ./reasonix_mobile_backend.sh log
```

Restart:

```bash
bash ./reasonix_mobile_backend.sh restart
```

Stop:

```bash
bash ./reasonix_mobile_backend.sh stop
```

---

# 25. Главная текущая цель

Не переписывать историю.

Текущая цель:

> Довести Reasonix Mobile APK 1.0 до реально работающего состояния на Android пользователя, проверить связку APK → mobile bridge → Reasonix/Balance → DeepSeek, затем протестировать Chat / Activity / Models / Budget / Skills / MCP Tools / Projects / Permissions и исправлять реальные баги в v1.0.x.

---

END OF HANDOFF
