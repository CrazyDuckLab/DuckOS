# DuckOS Xray Center

Status: development branch `develop/xray-center`, baseline `41921c3`
(develop/beta4-router-core). Nothing on this branch is published;
live qualification on Linux and against real clients is pending and
is called out explicitly below.

## Что это

Xray Center — подсистема полного жизненного цикла Xray в DuckOS:
серверы (inbounds), клиенты, подписки, REALITY/TLS, учёт трафика,
импорт и миграция. Она строится поверх существующих компонентов и
не создаёт параллельной привилегированной среды исполнения.

## Архитектура

Все крупные операции идут по существующей цепочке DuckOS:

    User Intent → Web UI / CLI → Public API (/api/v1/xray/*)
    → Engine → Agent (loopback /xray/v1/*) → Runtime (bounded exec)
    → Xray process → Observed state → Verification → Diagnostics

Ключевые модули (все в этом репозитории):

| Модуль | Назначение |
| --- | --- |
| `internal/xray/server.go` | Типизированная модель: Server/User/Profile/Subscription, протоколы vless/vmess/trojan/shadowsocks/socks/http, транспорты tcp/ws/grpc/xhttp/httpupgrade/kcp |
| `internal/xray/compatibility.go` | Версионная матрица совместимости Xray-core 26.3.27: протокол × транспорт × безопасность, flow, шифры SS, режимы XHTTP |
| `internal/xray/render.go` | Строгий рендер Xray JSON; секреты — только плейсхолдеры vault |
| `internal/xray/resolver.go` | Материализация vault-секретов перед `xray run -test` (TLS-файлы, REALITY privateKey/shortIds, креды клиентов в id/password/pass) |
| `internal/xray/reality.go` | CSPRNG-генерация X25519-пар, short id, UUID, паролей |
| `internal/xray/sharelink.go` | Resolved share-links (vless/trojan/vmess/ss) и строгий парсер URI импорта с сохранением неизвестных параметров |
| `internal/xray/stats.go` | Accountant: накопление дельт трафика, устойчивое к рестартам, stale-состояние, ограниченный ledger |
| `internal/xray/api.go` | Loopback CRUD API с apply-on-mutation и снапшотами каталога |
| `internal/subscription/delivery.go` | Публичная доставка `/sub/<opaque-token>`: токены только в виде SHA-256-дайджестов, единообразный 404, rate limit, ротация и отзыв |
| `internal/ximport/threeui.go` | Миграция из 3x-ui: dry-run маппинг в черновики, отчёт о неподдерживаемых полях |
| `internal/ximport/subfetch.go` | Импорт внешних подписок: только HTTPS, отказ приватных адресов после резолва (DNS rebinding закрывается), лимиты размера/времени/редиректов |

## Матрица совместимости (Xray-core 26.3.27)

| Протокол | Транспорты | Безопасность | Примечания |
| --- | --- | --- | --- |
| vless | tcp, ws, grpc, xhttp, httpupgrade, kcp | none, tls, reality | REALITY только tcp/xhttp; flow `xtls-rprx-vision` только vless+tcp+REALITY |
| vmess | tcp, ws, grpc, xhttp, httpupgrade, kcp | none, tls | alterId фиксирован 0; scy auto |
| trojan | tcp, ws, grpc, xhttp, httpupgrade, kcp | none, tls | |
| shadowsocks | tcp, ws, grpc, xhttp, httpupgrade, kcp | none, tls | AEAD и 2022-шифры; пароль на клиента |
| socks / http | tcp | none | учётные записи user/pass |

Матрица держится в `internal/xray/compatibility.go`; веб-зеркало —
`APIClient.xrayRealityAllowed/xrayFlowAllowed` в `web/src/api/client.ts`.
При обновлении пина Xray-core (`scripts/stage_xray.sh`) матрица
пересматривается и проходит повторную квалификацию.

## Подписки

- Доставка: `GET /sub/<token>` (по умолчанию; префикс настраивается).
  Токены 32 байта CSPRNG, хранятся только как SHA-256; ротация убивает
  прежние URL; у подписки независимый отзыв. Ошибки всех видов —
  одинаковый 404. Заголовки: `no-store`, `Profile-Update-Interval`.
  IP-доверие к прокси-заголовкам сознательно отключено.
- Форматы: base64-подписка из vless://, trojan://, vmess:// (v2 JSON),
  ss:// (SIP002). QR — на данных подписки (круговая проверка
  `decoded == rendered` закреплена тестами DV04).
- Доставка не зависит от административного UI (требование Core/Complete).

## Учёт трафика

Счётчики Xray сбрасываются при рестарте процесса, поэтому Accountant
хранит только накопленные дельты. Неудачный сбор помечает состояние
stale — устаревшие цифры никогда не подаются как живые. Продакшн-сбор
— `xray api statsquery` (пинованный бинарник, ограниченное окружение,
таймаут 5 c). Жёсткое применение квот требует verified-механизма и
сейчас не заявляется.

## Импорт и миграция

- 3x-ui: `duck xray import-3xui --file export.json` — read-only
  preview; приватные ключи REALITY не копируются в состояние, а
  помечаются для vault (purposes `reality-private`,
  `reality-shortid`); неподдерживаемые поля попадают в отчёт.
- Внешние подписки: `duck xray import-uri --url https://…` —
  SSRF-защита, строгий разбор строк, дубликаты и мусор — в отчёт.

## CLI

`duck xray servers|users|profiles|subscriptions list|get|create|delete`
плюс offline-операции `reality keygen`, `import-3xui`, `parse-uri`,
`import-uri`. CLI не обходит семантику публичного API.

## Фаза 3 (develop/xray-center, c 01c880a) — интеграция

- **Персистентность** (`internal/persist`): атомарные 0600-JSON-документы
  с fsync и откатом in-memory при сбое; узлы, группы, токены доставки,
  политики и события переживают рестарт (отзыв — тоже).
- **Реальный транспорт узлов** (`internal/xnode/transport.go`):
  NodeHTTPAdapter (HTTPS, bearer, опциональный pin sha256 отпечатка,
  заменяющий CA-цепочку) и NodeServer на стороне узла; защита от
  replay (timestamp + одноразовые nonce, окно ±2 мин); AdoptIdentity
  принимает креденшал ровно один раз.
- **Center API** (`internal/center`): nodes/groups/tokens/policies/
  stats/backup/events/diagnostics под `/api/v1/xray/center/*` через
  существующий аутентифицированный прокси; выключенная подсистема
  отвечает честным 501.
- **Публичная доставка**: duck-api монтирует `/sub/<token>` с общей
  таблицей токенов; рендеринг — через agent IPC от сервис-принципала
  `duck-api-delivery` (минимальные права), операторская сессия не
  нужна, vault из публичного демона недоступен.
- **Enforcement**: минутный цикл Enforcer — сбор stats (subprocess
  `xray api statsquery`), Accountant, решения квот, suspend в каталоге
  (пользователь уходит из рендера при следующем apply), события.
- **Diagnostics** (`internal/xray/diagnostics.go`): структурированные
  находки (код/серьёзность/свидетельства/ремедиация) для inbound,
  пользователя, статистики и флота.
- **E2E-workflows A–F** (`internal/center/workflow_test.go`): сквозные
  приёмочные сценарии на реальных durable-каталоге, рендере, доставке
  и транспорте; live-исполнение Xray-core — отдельная квалификация.

## Фаза 2 (develop/xray-center, c 1fb266a)

- **Multi-node** (`internal/xnode`): одноразовые enrollment-токены
  (хранятся как SHA-256), креденшалы узлов показываются один раз,
  ротация и терминальный отзыв, staleness выводится из часов,
  Reconcile с изоляцией отказа на узел, Adapter для транспорта.
- **Подписки**: форматы base64/plain/Clash-YAML/JSON (`?format=`),
  упорядоченные группы с клиентскими именами узлов
  (`internal/subscription/group.go`).
- **TLS** (`internal/certs/inventory.go`): инвентарь на реальном
  PEM-парсинге, классы valid/expiring/expired, renewal-автомат
  pending/issued/failed с ограниченной диагностикой; исполнение
  ACME-challenge — отдельная сетевая квалификация.
- **REALITY** (`internal/xray/realitymgr.go`): инвентарь публичных
  записей, ротация без разрыва клиентов, жизненный цикл short id,
  preview клиентских параметров.
- **Квоты** (`internal/xray/quota.go`): окна сброса day/week/month,
  решения ok/warn/exhausted, auto-suspend с явным флагом Enforced —
  это не жёсткое per-packet ограничение.
- **Политики маршрутизации** (`internal/xray/policy.go`):
  детерминированная компиляция, обнаружение конфликтов, blackhole
  только по ссылке, отказ маршрутизации в неизвестный outbound.
- **Insights** (`internal/xray/insights.go`): кольцо событий
  (1024 шт., 7 суток) только реальных наблюдений с источником.
- **Бэкап** (`internal/xray/backup.go`): экспорт без секретов по
  умолчанию, проверка ссылочной целостности, preview
  add/update/conflict, транзакционный restore.
- **Web UI**: P050 заменён на функциональную страницу Xray Center
  (создание/удаление серверов, клиентов, подписок, матрица
  совместимости, офлайн-help EN/RU/DE/ES, confirm-диалоги).

## Фаза 4 (develop/xray-center, c dac2f62) — runtime-квалификация

Квалификация выполнялась **реальным бинарником Xray-core 26.3.27**
(darwin/arm64 сборка той же версии; linux-бинарники пинятся sha256 в
`scripts/stage_xray.sh`). Набор `internal/qa/xraycore` (пропускается
без бинарника):

- Матрица `xray run -test`: 30 комбинаций — 4 протокола × 6
  транспортов (security=none), VLESS+REALITY (tcp, xhttp),
  VLESS+TLS (tcp, ws), SOCKS и HTTP — **все проходят**.
- Живой цикл: запуск xray с SOCKS-inbound → реальный HTTP-трафик
  через прокси (curl) → наблюдение через `xray api statsquery`
  (user-счётчики, инкремент подтверждён) → завершение.
- Отказ аутентификации: неверный SOCKS-креденшал не создаёт сессию.
- Invalid-config: `xray -test` отвергает битый конфиг (пин отката).

Реальные находки фазы: (1) 26.3.27 требует шифр SS в **каждой**
client-записи — рендер исправлен; (2) per-inbound счётчики не
отдаются под policy.levels — enforcement корректно опирается на
user-счётчики; (3) дедлок персистентности реестра узлов (отзыв узла
вешал бы агента) — исправлен; (4) принципал доставки ограничен
одним preview-эндпоинтом (10 bypass-сценариев покрыты тестами).

## Ремедиация UQ-01 (P1, безопасность персистентности)

Подтверждённый аудитом дефект: хук персистентности xnode отбрасывал
ошибку — неудачная запись на диск сопровождалась успешным ответом на
отзыв, и после перезапуска отозванный креденшал воскресал.

Исправление — транзакционные мутации: validate → мутация в памяти
под локом → коммит ПОЛНОГО durable-состояния (узлы + enrollment-
токены) → при ошибке откат в памяти и возврат ошибки с причиной
(ENOSPC/EACCES/EIO проходят до CLI/HTTP: center API отдаёт
503 `revocation_not_committed`/`rotation_not_committed`). Покрыты:
отзыв узла, ротация креденшала, suspend/resume, enrollment (burn
одноразового токена теперь durable — ранее жил только в памяти),
выдача enrollment-токенов, выпуск/ротация/отзыв подписных токенов,
upsert/delete групп. Консультативные записи (last-seen, use-счётчики)
остаются best-effort осознанно.

Регрессионный набор UQ-01 A–J (`internal/xnode/durability_test.go`,
`disk_test.go`; `internal/subscription/durability_test.go`):
fault-инъекция с проверкой РЕАЛЬНЫХ решений аутентификации после
реконструкции менеджера, переживание рестарта на реальном
persist.File-адаптере (атомарный rename + fsync, 0600), идемпотентный
retry, конкурентные отзывы без потерянных обновлений (race).

## Ремедиация UQ-02 (P1, post-rename commit uncertainty)

Дефект: отказ directory-fsync ПОСЛЕ успешного rename трактовался как
обычный сбой — память откатывалась к предыдущему (активному)
состоянию, и следующая мутация записывала активный снапшот поверх
отозванного, воскресая креденшал после рестарта.

Модель исходов коммита (`internal/persist/commit.go`):
`CommitOK` / `CommitNotCommitted` (до rename — старый файл
авторитетен, откат памяти) / `CommitUncertain` (после rename — новые
байты видимы; память СОХРАНЯЕТ ограничительное состояние).

Fail-closed при неопределённости: домен помечается DEGRADED, все
последующие мутации отказываются (запись поверх неопределённого
снапшота могла бы воскресить отзыв), операция возвращает отдельный
UNCERTAIN-неуспех. Монотонный `DurableState.Revision` — якорь
реконсиляции (`POST /center/storage/reconcile`, статус —
`GET /center/storage`): равная ревизия = коммит лёг на диск; старее —
диск принимается как авторитетный с явной директивой перепроверить
недавние отзывы; новее — отказ.

Единый атомарный снапшот `xnode-state.json` заменил
последовательную запись двух файлов; миграция legacy-каталога —
чистая валидирующая конверсия (`MigrateLegacy`), legacy-файлы
сохраняются как доказательство отката и не пишутся больше никогда.

Ограничение честности: физическая durability при потере питания на
этом хосте не демонстрируется — требуется Linux-квалификация
(duck-dev-01, seccomp-инъекция на реальной границе fsync); граница
fsync в тестах инъектируется на реальном системном пути.

## Ремедиация аудита от 2026-10-19 (7 находок)

AUD-01: статус/реконсиляция хранилища подключены к установленному
агенту (`GET /center/storage`, `POST /center/storage/reconcile`,
CLI `duck xray storage status|reconcile`). AUD-02: выпуск подписных
токенов транзакционен (откат при NOT_COMMITTED, DEGRADED при
Uncertain). AUD-03: реконсиляция — рестриктивный merge (отзывы и
потребление токенов монотонны, новейший хэш креденшала побеждает).
AUD-04: durable migration boundary (`xnode-migrated.json`); утрата
или порча снапшота после миграции = fail closed, legacy карантин
(`internal/nodestate`). AUD-05: pre-lock degraded-гейт брал мьютекс
неправильно — race подтверждён детектором и устранён. AUD-06:
GroupStore — degraded-гейт и лимит 256 в критической секции мутации.
AUD-07: delivery-демон более не держит таблицу токенов — каждый
запрос валидируется авторитетно через restricted IPC
(`delivery-validate`, принципал ограничен preview+validate), отказ
IPC = fail-closed 404; отзыв/ротация мгновенны. Все 7 регрессионных
тестов верифицированно падают на уязвимом коде (сохранён в
аудиторском worktree) и проходят после ремедиации.

## Ремедиация AUD-04N (свежая установка, рестарт)

Дефект: чистая установка коммитила маркер миграции, но не пустой
`xnode-state.json`, а «живость» снапшота определялась по содержимому
— незатронутая установка падала fail-closed на втором старте
(недоступен весь Xray Center, включая подписки). Исправление:
строгий порядок инициализации — сначала авторитетный снапшот
(валидный пустой для fresh install, полный протокол durability),
затем маркер; «живость» — по СУЩЕСТВОВАНИЮ файла
(`persist.File.Exists`); прерывания и uncertain-коммиты
инициализации восстанавливаются на следующем старте; утрата
снапшота при живом маркере по-прежнему fail closed. Сценарии A–I —
PASS под `-race`; на логике af88dcc сценарий A воспроизведён
верифицированным FAIL.

## Стабилизация 2026-10-20 (R-01…R-05)

R-01: реконсиляция соблюдает приоритет REVOKED > SUSPENDED > active
(приостановленный узел не реактивируется старым снапшотом; легитимный
durably-committed resume не блокируется). R-02: реконсиляция токенов
и групп — рестриктивный merge с durable-надгробиями (отзыв переживает
утрату записи на диске; disabled-группы не реактивируются; expiry
неизменяем). R-03: реконсиляция читает авторитетный диск
(`persist.File.Reload`); нечитаемый снапшот держит домен degraded.
R-04: точка линеаризации авторизации подписки — вторая валидация
после рендера до первого байта в сеть; уже переданные данные
ретроспективно не отзываются (задокументировано). R-05: гейт
DEGRADED и мутация — одна критическая секция (IssueEnrollmentToken,
RotateCredential, TokenStore.Issue); детерминированный TOCTOU-тест.
NodeServer: rate limit 120/мин на пир, ≤8 конкурентных запросов,
дедлайн 30 с.

## Продуктовая Multi-Node проводка (2026-10-20, продолжение)

Node-роль интегрирована в установленный продукт: флаги агента
`--node-control-listen/--node-control-cert/--node-control-key`
(ВЫКЛЮЧЕНО по умолчанию; без provisioned TLS-пары листенер
отказывается стартовать; публичный листенер не открывается
неявно). Постоянная идентичность узла — `node-identity.json`
(первичное принятие, constant-time проверка). Протокол ротации из
трёх фаз (deliver → commit; pending-слот переживает потерю ack;
commit чужого креденшала отвергается; старый креденшал умирает при
коммите). Координатор: `POST /center/nodes/redeem` (одноразовый
токен, креденшал показывается один раз) + CLI `duck xray nodes
redeem`. Pushed-конфигурация узла идёт через тот же
Runtime.Apply safe-apply путь. Интеграционные тесты — полный цикл
ротации через настоящий TLS с durable-хранилищами обеих сторон.
Ограничение: разделение по ОС-процессам и live-квалификация —
после Linux-lab.

## Multi-Node E2E и Linux-квалификация (2026-10-20, продолжение 2)

**OS-процессный E2E** (контроллер и узел — раздельные процессы,
раздельные durable-каталоги, pinned TLS): полный цикл (enrollment →
redeem-семантика → отказ неверного пина/креденшала → discovery →
health → push → рестарт процесса узла с сохранением идентичности →
трёхфазная ротация → отказ старого креденшала на ОБЕИХ сторонах →
персистентность ревокации при рестарте контроллера) — PASS локально
и **нативно на duck-dev-01 (linux/amd64)**.

**Linux RUNTIME VERIFIED**: полный runtime-набор против реального
пинованного Xray-core 26.3.27 linux/amd64 (sha256 по stage-пину) на
duck-dev-01 — матрица 30/30, SOCKS-loop со статистикой, отказ
аутентификации, reject невалидного конфига.

**Матрица сбоев ротации**: рестарт контроллера до коммита (retry со
свежим креденшалом; брошенный креденшал никогда не приемлем),
replay старого креденшала — отказ на обеих сторонах, постоянный
партишн до доставки (fail-safe: узел недостижим, но не небезопасен),
дубли доставки/подтверждения идемпотентны (Commit для уже
закоммиченного — no-op). Точки линеаризации авторизации и
tradeoff pending-слота задокументированы в заголовке теста.

Чекпоинт оставшихся работ (не PASS): Nodes UI (API/CLI готовы),
ACME-challenge, live-подключения 30 комбинаций реальными клиентами,
установочный clean-install прогон.

## Честный статус

IMPLEMENTED AND TESTED (unit/race-тесты на darwin, полный
`GOOS=linux` build+vet): модель, матрица, рендер, резолвер,
share-links, парсер URI, REALITY keygen, доставка подписок,
миграция 3x-ui, SSRF-импорт, Accountant, CLI, веб-клиент.

IMPLEMENTED, LIVE QUALIFICATION PENDING: применение новых
протоколов/транспортов реальным процессом Xray на Linux
(`internal/qa`, NETWORK_REQUIRED), сбор статистики против живого
stats-слушателя, доставка `/sub` на публичном HTTPS-домене с
боевыми клиентами (v2rayN/v2rayNG/Streisand), квалификация на
ARMv7/ARM64.

NOT IMPLEMENTED: multi-node оркестрация и enrollment, ACME- issuance,
политики маршрутизации поверх safe-apply, полноценные формы
создания/редактирования в Web UI (сейчас — capability-зеркало и
полный API-клиент), Help Everywhere, бэкап/рестор каталога Xray.

BLOCKED: ничего.
