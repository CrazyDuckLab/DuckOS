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
