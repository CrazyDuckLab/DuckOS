# Localization and contextual help

## Four-language policy

English (en), Russian (ru), German (de), Spanish (es) are first-run choices and later account/browser preferences. Select language before local registration. Do not send a language selection to any external service. Locale changes presentation, not networking semantics, backend enum values, IDs, decimal wire formats or CLI commands.

Protocol tokens remain canonical: VLAN, QinQ, VRF, BGP, OSPF, IS-IS, RIP/RIPng, BFD, NAT/DNAT/SNAT, WireGuard, Xray, VLESS, Trojan, REALITY, IPsec, MPLS, EVPN, VXLAN, RPKI, LLDP, STP/RSTP/MSTP, DHCP/DHCPv6, DNS, DoH/DoT, QoS, DSCP, MTU/PMTU/MSS, SFP, PoE, RADIUS, 802.1X, API, CLI. Translate the surrounding grammar and explanations. Protocol state code may remain in a technical column; explain it locally (BGP Active is an FSM state, not a claim that the peer works).

The research terminology samples retain acronyms while translating prose (RESEARCH sources 27–29). They include old/translated documentation and are not a universal glossary. A network-literate native reviewer is required for each locale.

## Language voice

- EN: direct verbs, short specific sentences, no “simply/just.” “This changes…” before risky actions. Prefer “Blocked by rule …” to “Connectivity issue.”
- RU: professional natural Russian, no informal “ты,” use concise infinitive button verbs. “Маршрут,” “шлюз,” “межсетевой экран” in prose; keep NAT/VRF and protocol terms. Explain “кандидат конфигурации” initially as a draft of changes. Avoid declining an opaque ID or inventing Russian protocol acronyms.
- DE: concise technical German, neutral infinitive buttons; use “Routing,” “Firewall,” “VLAN” and “Schnittstelle.” Formal Sie in instructional prose if direct address is needed. Avoid monster compounds by splitting clauses, not clipping text.
- ES: neutral international Spanish; “interfaz,” “enrutamiento,” “puerta de enlace,” “regla,” with canonical protocol acronyms. Infinitive action labels; no region-specific slang. Never translate literal CLI paths.

Do not translate human aliases entered by administrators. Isolate technical tokens from localized bidirectional text; all V1 locales are LTR but string rendering should be safe with arbitrary names. Validate Unicode/control characters according to backend policy and escape untrusted content. Help and backend errors are rendered as text, not injected HTML.

## Core glossary and action distinctions

| Concept | EN | RU | DE | ES |
|---|---|---|---|---|
| Interfaces | Interfaces | Интерфейсы | Schnittstellen | Interfaces |
| WAN | WAN | WAN | WAN | WAN |
| LAN / VLAN | LAN / VLAN | LAN / VLAN | LAN / VLAN | LAN / VLAN |
| Routing | Routing | Маршрутизация | Routing | Enrutamiento |
| Firewall | Firewall | Межсетевой экран | Firewall | Firewall |
| Tunnels | Tunnels | Туннели | Tunnel | Túneles |
| DNS / DHCP | DNS / DHCP | DNS / DHCP | DNS / DHCP | DNS / DHCP |
| Services | Services | Сервисы | Dienste | Servicios |
| System | System | Система | System | Sistema |
| Logs / Diagnostics | Logs / Diagnostics | Журналы / Диагностика | Protokolle / Diagnose | Registros / Diagnóstico |
| Stage | Add to candidate | Добавить в набор изменений | Zum Entwurf hinzufügen | Añadir al borrador |
| Preview | Preview changes | Предпросмотр изменений | Änderungen prüfen | Revisar cambios |
| Apply temporarily | Apply with rollback protection | Применить с защитой откатом | Mit Rückfallschutz anwenden | Aplicar con reversión automática |
| Confirm | Confirm changes | Подтвердить изменения | Änderungen bestätigen | Confirmar cambios |
| Roll back now | Roll back now | Откатить сейчас | Jetzt zurücksetzen | Revertir ahora |
| Discard draft | Discard draft | Отменить черновик | Entwurf verwerfen | Descartar borrador |
| Save preference | Save | Сохранить | Speichern | Guardar |
| Desired | Configured | Настроено | Konfiguriert | Configurado |
| Observed | Observed | Наблюдается | Beobachtet | Observado |
| Disabled intentionally | Disabled by administrator | Отключено администратором | Administrativ deaktiviert | Desactivado por el administrador |
| Unknown | Not enough evidence | Недостаточно данных | Unzureichende Nachweise | Datos insuficientes |
| Stale | Observation is outdated | Данные устарели | Beobachtung veraltet | Datos desactualizados |
| Unsupported | Not supported here | Здесь не поддерживается | Hier nicht unterstützt | No compatible aquí |
| Hardware pending | Hardware required | Требуется оборудование | Hardware erforderlich | Hardware necesario |
| Credential pending | Credentials required | Требуются учётные данные | Zugangsdaten erforderlich | Credenciales necesarias |
| Managed by | Managed by | Управляется: | Verwaltet durch | Gestionado por |
| Explain Why | Why? | Почему? | Warum? | ¿Por qué? |
| Show CLI | Show CLI | Показать CLI | CLI anzeigen | Mostrar CLI |
| Show API | Show API | Показать API | API anzeigen | Mostrar API |
| Test | Test | Проверить | Testen | Probar |
| Capture | Capture packets | Захватить пакеты | Pakete erfassen | Capturar paquetes |
| Show QR | Show QR | Показать QR | QR anzeigen | Mostrar QR |
| Copy config | Copy configuration | Скопировать конфигурацию | Konfiguration kopieren | Copiar configuración |
| Download config | Download .conf | Скачать .conf | .conf herunterladen | Descargar .conf |
| Rotate keys | Rotate keys | Сменить ключи | Schlüssel wechseln | Rotar claves |
| Read-only | Read-only access | Только просмотр | Nur Lesezugriff | Solo lectura |
| Conflict | Revision conflict | Конфликт версий | Versionskonflikt | Conflicto de revisiones |
| Readiness | Feature availability | Доступность функции | Funktionsverfügbarkeit | Disponibilidad de funciones |

A “confirmed” commit is not a “saved preference”; a “reverted draft” is not a kernel rollback. Keep action distinctions even when translating creates longer buttons.

## Message architecture

Stable IDs such as `wan.probe.dns.failed`, `transaction.awaitingConfirmation`, `export.warning.privateKey`, `policy.explain.matched`. Typed parameters include object references, counts, durations, protocol and port. No behavior branches on localized strings. Map known backend reason codes; unknown code receives a safe general explanation plus copyable diagnostic ID and sanitized technical detail.

Use complete sentences with named parameters and ICU/CLDR-compatible plural handling, not concatenated fragments. RU needs one/few/many/other; do not assume a two-form English plural. All locales format durations/numbers through one formatter. IP/MAC/ASN/prefix/port and exact CLI remain locale-invariant. Decimal API values use protocol-standard encoding even if display decimal separator differs.

Do not persist translated backend errors or Explain Why prose as canonical state. Store reason IDs and parameters where backend supports it. Where current backend returns only English text, show a localized summary plus sanitized “Technical detail (English)” until structured reasons exist; never silently mistranslate security meaning.

Date/time display distinguishes node time, browser display timezone and monotonic duration. “Observed 12 s ago” only if the age is defensible. TIME_UNTRUSTED explains which events cannot be ordered reliably; IDs/sequences remain useful.

## Approved high-risk message examples

| ID | EN | RU | DE | ES |
|---|---|---|---|---|
| publish.preview | This will open {protocol} {port} on {wan} and forward it to {target}. | Будет открыт {protocol} {port} на {wan} с перенаправлением на {target}. | Dies öffnet {protocol} {port} auf {wan} und leitet den Verkehr an {target} weiter. | Se abrirá {protocol} {port} en {wan} y se reenviará el tráfico a {target}. |
| export.warning.privateKey | Anyone who scans this code can use this connection. | Любой, кто отсканирует этот код, сможет использовать подключение. | Wer diesen Code scannt, kann diese Verbindung nutzen. | Cualquier persona que escanee este código podrá usar esta conexión. |
| transaction.outcomeUnknown | The result is not yet known. Checking transaction {id}; do not apply again. | Результат пока неизвестен. Проверяется транзакция {id}; не применяйте изменения повторно. | Das Ergebnis ist noch unbekannt. Transaktion {id} wird geprüft; nicht erneut anwenden. | El resultado aún se desconoce. Se comprueba la transacción {id}; no vuelva a aplicar los cambios. |
| profile.refreshRequired | Configuration changed. The remote client must import revision {revision}. | Конфигурация изменилась. На удалённом клиенте нужно импортировать версию {revision}. | Die Konfiguration wurde geändert. Der entfernte Client muss Version {revision} importieren. | La configuración ha cambiado. El cliente remoto debe importar la revisión {revision}. |
| time.untrusted | Node time is not trusted. Certificate validity cannot yet be verified. | Время узла недостоверно. Пока нельзя проверить срок действия сертификата. | Die Gerätezeit ist nicht vertrauenswürdig. Die Zertifikatsgültigkeit kann noch nicht geprüft werden. | La hora del nodo no es fiable. Aún no se puede verificar la validez del certificado. |
| support.unavailable | This build does not expose this operation. Existing configuration was not changed. | В этой сборке операция недоступна. Текущая конфигурация не изменена. | Dieser Build stellt den Vorgang nicht bereit. Die bestehende Konfiguration wurde nicht geändert. | Esta compilación no ofrece esta operación. No se modificó la configuración existente. |

Localization includes navigation, all help, validators, dialogs, errors, status explanations, empty states, ARIA labels, chart summaries, update/release summaries and notifications. Downloaded standard configurations are not translated. Release-note prose received from upstream needs a labeled source language/fallback, not an invented translation treated as signed content.

## Layout and quality gates

Allow 40% expansion and long compound terms; wrap labels/help and grow dialogs/strip heights. Never ellipsize the only dangerous-action consequence. Table columns resize; complete values remain available in details/copy. Test plural counts 0,1,2,4,5,11,21,101; IPv6; long aliases; empty strings; locale switch during pending transaction.

All keys exist in en/ru/de/es. CI rejects missing keys, mismatched parameter sets, literal UI strings outside explicitly canonical allowlist and translated enum use. Native reviewers evaluate high-risk instructions and protocol usage. Pseudo-localization is a test tool, never a supported production language.

## Contextual help system

Every page header and complicated group has a labeled Help/Info control with a concise inline hint, a nonmodal expanded panel and an optional local documentation link. Help opens with the object's context but never sends secrets to a service. Experts may collapse hints; Help remains discoverable. Offline help is bundled and revisioned with DuckOS.

Each topic answers seven questions:

1. What is it?
2. When should I use it?
3. What changes?
4. What can break?
5. What depends on it?
6. A practical example.
7. How do I diagnose it?

Help is structured content keyed by `help.<pageId>.<section>`, with title, summary, effect, risk, dependencies, example and diagnostics. Link labels name the destination. Any example addresses are documentation examples, never pre-applied default configuration. The page contracts supply domain questions, fields, risk and diagnostics; below are approved expanded examples for the highest-confusion topics.

| Topic | What / when | Effect and risk | Dependencies / example | Diagnose |
|---|---|---|---|---|
| VRF | Independent routing context for tenants or overlapping networks | Assigning an interface changes which routes it uses; routes do not cross automatically | VRF → members → route filters/leaks → firewall. Example: Staff and Lab keep separate tables | Lookup in the selected VRF; inspect leaks and access separately |
| VLAN/tagging | Separates L2 networks on shared links | Wrong untagged/PVID/trunk membership can cut off management | Port → bridge → VLAN → DHCP/access. Example: VLAN30 guest on tagged uplink | Compare ingress tag, PVID and egress membership; check DHCP on that VLAN |
| QinQ | Carries an inner client tag inside an outer service tag | Mixing tag roles can join or isolate wrong networks | Outer S-tag and inner C-tag named separately; peer/provider agreement needed | Inspect captured tags and MTU with supported tooling |
| Firewall direction | Protect router itself or control transit between networks | An allow to router is not an allow to a LAN server | Example: Staff → NAS → SMB differs from Staff → DuckOS management | Trace the packet hook/context and matching rule |
| NAT | Changes addresses/ports; it is not a permission by itself | A DNAT target still needs matching access and a usable return path | Example WAN1 TCP443 → 192.168.10.20:443 plus visible allow rule | Test from outside; inspect original/reply tuples and return route |
| Hairpin / split DNS | Lets local clients reach a local service by its public name | Hairpin changes local packet handling; split DNS changes answers by view | Pick intentionally; neither proves external reachability | Compare local DNS answer and path with external-vantage test |
| BGP filters | Accept/reject/modify routes from or to a peer | A wrong export policy can advertise unintended prefixes | Peer → import/export filter → RIB/FIB. Start with explicit allowed prefixes | View accepted/rejected/advertised routes and exact term trace |
| Route preference | Selects among eligible routes in a specific context | Preference and metric are different and protocol-dependent | Explain selected protocol/table/next hop; do not guess best path | Show provided selection reason and recursion, not only shortest line |
| CGNAT | Provider may share an upstream public address | Publishing may fail even if local NAT is configured | Evidence may be a private/shared WAN address plus outside address observation | Show detection confidence; external reachability test; discuss supported alternatives |
| Split DNS | Different domains/clients use different answer sources | Circular forward dependencies or unexpected views can break lookup | Client network → view → upstream/zone → answer | Query with the actual client/view context and show answer source |
| IPv6 PD | ISP delegates a prefix for downstream networks | Prefix change/expiry affects LAN, firewall, DNS and tunnel exports | WAN PD → per-LAN assignments → RA/DHCPv6 | Verify prefix lifetime, advertisement and IPv6 reachability separately |
| MTU/PMTU/MSS | Limits packet size and detects path-size problems | Too small hurts efficiency; too large may black-hole traffic; MSS only affects TCP | Auto first, manual advanced; tunnel encapsulation affects usable MTU | Scoped PMTU probe, ICMP evidence, TCP vs UDP distinction |
| ACME | Automates certificate issuance/renewal through validation | DNS/HTTP challenge, time or credentials may prevent renewal | Names → challenge → CA → certificate → listeners | Check challenge evidence, rate limit, time trust and current certificate |
| Safe Apply | Temporarily applies consequential changes with rollback protection | Confirming too early accepts a broken path; browser timer is not authority | Candidate → management checks → backend watchdog → receipt | Inspect transaction ID/phase, evidence and actual rollback receipt |
| Offload | Hardware may process packets outside normal software path | Some rules/counters/capture may not see offloaded traffic | Driver/chip capability → interfaces → firewall coverage | Compare requested/effective mode and enforcement evidence |
| Generated objects | Automation creates ordinary objects with an owner | Manual changes may conflict with the generator; detach needs impact review | Publish Service owns DNAT + allow; both remain in normal tables | Follow Managed by and inspect desired/generated/observed differences |
| Secret QR | Encodes a usable credential/configuration | Anyone receiving it can use the granted access | Same revision and bytes as download; regeneration after key/access changes | Decode in tests, show refresh-required state, revoke/rotate if exposed |
| TIME_UNTRUSTED | Node lacks trusted wall-clock evidence | Certificate/expiry decisions may be unavailable or unsafe | NTP/NTS/PTP/source evidence; local operation may still work | Inspect sources and trust reason without pretending current time is known |
| Logging fallback | Keeps a small amount of recent evidence if external storage fails | Unbounded fallback can fill the OS and harm routing | Tiny internal cap, dropped count and alert; protected audit separate | Inspect sink failure, quota/reserve and effective buffer bounds |
| Single-NIC | A server can provide firewall/tunnel/services without a separate LAN | Current public management path must be preserved | One NIC → host firewall → services/tunnels; later add LAN deliberately | Distinguish router-origin/host traffic from forwarded traffic |

Every other page receives the same seven-part help structure, populated with its catalog-specific purpose, fields, dependencies, risk and diagnostic actions. This is content architecture, not permission to ship identical generic paragraphs on all pages. A page is incomplete until its localized domain help has been reviewed against its actual backend.
