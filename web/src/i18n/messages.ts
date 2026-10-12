/**
 * DuckOS V1 — Localization registry.
 *
 * All UI strings flow through `t()` so every locale stays in lock-step with
 * English. Missing keys fall back to English, never to a blank string — that
 * way a half-translated screen still tells the truth.
 *
 * Locale identity is `language-REGION` only when a region is required
 * (today: `de-DE`, `es-ES`, `ru-RU`); `en` is the bare code.
 */
export type Locale = "en" | "ru" | "de" | "es";

export const LOCALES: ReadonlyArray<{ id: Locale; label: string }> = [
  { id: "en", label: "English" },
  { id: "ru", label: "Русский" },
  { id: "de", label: "Deutsch" },
  { id: "es", label: "Español" },
];

/**
 * String bundle type. Every key must exist in every locale file. Adding a key
 * means adding it to *all* four bundles; the type system will refuse the build
 * otherwise.
 */
export interface Messages {
  appName: string;
  appTagline: string;
  brandShort: string;

  shell: {
    searchPlaceholder: string;
    searchHint: string;
    searchSectionHint: string;
    modeEasy: string;
    modeFull: string;
    modeEasyDescription: string;
    modeFullDescription: string;
    systemMenu: string;
    signOut: string;
    signIn: string;
    time: string;
    timeTrusted: string;
    timeUntrusted: string;
    timeUntrustedHint: string;
    theme: string;
    themeSystem: string;
    themeLight: string;
    themeDark: string;
    language: string;
    online: string;
    offline: string;
    degraded: string;
    pending: string;
    candidate: string;
    apply: string;
    discard: string;
    review: string;
    showCli: string;
    showApi: string;
    explainWhy: string;
    inspection: string;
    copyCli: string;
    copyApi: string;
    closeInspector: string;
    primary: string;
    question: string;
    columns: string;
    filters: string;
    runtime: string;
    diagnostics: string;
    easyHint: string;
    dependencies: string;
    risk: string;
    features: string;
    packages: string;
    pinned: string;
    noResults: string;
    loading: string;
    empty: string;
    error: string;
    unknown: string;
    unavailable: string;
    backToFull: string;
    mode: string;
    moreDetails: string;
    close: string;
    clear: string;
    ref: string;
    whatIsThis: string;
    easyModeHint: string;
  };

  actionbar: {
    noChanges: string;
    changesWaiting: string;
    validateAndApply: string;
    discardAll: string;
  };

  nav: {
    interfaces: string;
    wan: string;
    lan: string;
    routing: string;
    firewall: string;
    tunnels: string;
    dns: string;
    services: string;
    system: string;
    logs: string;
    easy: string;
  };

  health: {
    healthy: string;
    needsAttention: string;
    degraded: string;
    unknown: string;
    overall: string;
    overview: string;
    readiness: string;
    doctor: string;
    topology: string;
    apiReachable: string;
    apiUnreachable: string;
    openDoctor: string;
    activeIncidents: string;
    pendingChanges: string;
    none: string;
    timeTrusted: string;
    timeUntrusted: string;
  };

  overview: {
    title: string;
    description: string;
    issues: string;
    noIssues: string;
    cpu: string;
    memory: string;
    storage: string;
    uptime: string;
    interfaces: string;
    routes: string;
    dns: string;
    system: string;
    hostname: string;
    os: string;
    kernel: string;
    arch: string;
    observed: string;
    notObserved: string;
    internetEvidence: string;
    noInternetEvidence: string;
    /* v3 system overview additions */
    cpuHint: string;
    interfacesHint: string;
    uptimeHint: string;
    findingsTitle: string;
    identity: string;
    architecture: string;
    mount: string;
    filesystem: string;
    used: string;
    available: string;
    noRoot: string;
    shortcuts: string;
    topology: string;
    candidate: string;
    easy: string;
  };

  candidate: {
    title: string;
    empty: string;
    description: string;
    reviewTitle: string;
    reviewDescription: string;
    noChanges: string;
    blastRadius: string;
    confidence: string;
    owner: string;
    operation: string;
    before: string;
    after: string;
    canary: string;
    locks: string;
    conflicts: string;
    validate: string;
    apply: string;
    discard: string;
    expires: string;
    confirmWindow: string;
    noConfirmSupport: string;
  };

  page: {
    easyHomeTitle: string;
    easyInternetTitle: string;
    easyNetworksTitle: string;
    easyDevicesTitle: string;
    easyPublishTitle: string;
    easyCareTitle: string;
  };

  inspector: {
    title: string;
    description: string;
    primaryAction: string;
    contextActions: string;
    cliLabel: string;
    apiLabel: string;
    copyCli: string;
    copyApi: string;
    close: string;
    columnsHeader: string;
    columnsHelp: string;
    dependenciesHeader: string;
    noDeps: string;
  };

  /* — Common user-visible strings shared across many screens.
   *   These are the strings that would otherwise appear as hardcoded
   *   English in configurators, tables, dialogs, and the bottom dock. */
  common: {
    cancel: string;
    save: string;
    apply: string;
    discard: string;
    edit: string;
    delete: string;
    create: string;
    refresh: string;
    yes: string;
    no: string;
    none: string;
    all: string;
    more: string;
    back: string;
    next: string;
    close: string;
    clear: string;
    filter: string;
    loading: string;
    emptyRows: string;
    noResults: string;
    selectPage: string;
    observationStale: string;
    observationStaleHint: string;
    observationUntrusted: string;
    pendingTxStale: string;
    rowsOf: string;
    pageOf: string;
  };

  /* — Intent titles (Safe Apply dock + Candidate page). Each is what
   *   appears in the bottom dock for a pending change. */
  intent: {
    createInterface: string;
    editInterface: string;
    deleteInterface: string;
    createFirewallRule: string;
    editFirewallRule: string;
    deleteFirewallRule: string;
    createWireguardServer: string;
    editWireguardServer: string;
    deleteWireguardServer: string;
    editWireguardPeer: string;
    createXrayServer: string;
    editXrayServer: string;
    deleteXrayServer: string;
    createQosClass: string;
    editQosClass: string;
    deleteQosClass: string;
    createDnsZone: string;
    editDnsZone: string;
    deleteDnsZone: string;
    createDhcpScope: string;
    editDhcpScope: string;
    deleteDhcpScope: string;
    createAddress: string;
    editAddress: string;
    deleteAddress: string;
    createRoute: string;
    editRoute: string;
    deleteRoute: string;
    createRouteMap: string;
    editRouteMap: string;
    deleteRouteMap: string;
    createBridge: string;
    editBridge: string;
    deleteBridge: string;
    createBond: string;
    editBond: string;
    deleteBond: string;
    createVlan: string;
    editVlan: string;
    deleteVlan: string;
    createBgpPeer: string;
    editBgpPeer: string;
    deleteBgpPeer: string;
  };

  /* — Easy-mode phrase bank. Short copy used on the five Easy pages. */
  easy: {
    question5Title: string;
    question5Body: string;
    /* Easy v3 — product-level dashboard copy */
    home: {
      title: string;
      subtitle: string;
      internetGood: string;
      internetWarn: string;
      internetBad: string;
      internetUnknown: string;
    };
    internet: {
      title: string;
      hint: string;
      connected: string;
      degraded: string;
      connection: string;
      gateway: string;
      interface: string;
      dns: string;
      searchDomains: string;
      uplinks: string;
      noUplinks: string;
      configure: string;
      configureHelp: string;
      configureAction: string;
    };
    wifi: {
      title: string;
      hint: string;
      detail: string;
      configure: string;
    };
    networks: {
      title: string;
      none: string;
      wifi: string;
      wifiUnsupported: string;
    };
    devices: {
      title: string;
      hint: string;
      online: string;
      onlineHint: string;
      list: string;
      none: string;
      view: string;
    };
    vpn: {
      title: string;
      hint: string;
      detail: string;
      configure: string;
    };
    system: {
      title: string;
      hint: string;
      allGood: string;
      findings: string;
      openDoctor: string;
      about: string;
      hostname: string;
      uptime: string;
      environment: string;
    };
    publish: {
      title: string;
      hint: string;
      stepsTitle: string;
      step1: string;
      step1Help: string;
      step2: string;
      step2Help: string;
      step3: string;
      step3Help: string;
      currently: string;
      noSites: string;
      configureAction: string;
    };
    care: {
      title: string;
      hint: string;
      allGood: string;
      allGoodHint: string;
      findings: string;
      findingsHint: string;
      findingsTitle: string;
      actionsTitle: string;
      actionsHelp: string;
      openDoctor: string;
      backup: string;
      updates: string;
    };
    tile: {
      open: string;
      details: string;
    };
  };

  a11y: {
    openNavigation: string;
    closeNavigation: string;
    openInspector: string;
    skipToMain: string;
    navPrimary: string;
    navSection: string;
  };
}

/* English — canonical copy. Every other locale must mirror every key. */
const en: Messages = {
  appName: "DuckOS",
  appTagline: "Your network. Clearly.",
  brandShort: "DuckOS",

  shell: {
    searchPlaceholder: "Where do I go? Try a name, role, or section.",
    searchHint:
      "Search the workspace — not the live network. Press Enter to open the page.",
    searchSectionHint: "Matches a section, page, or task.",
    modeEasy: "Easy",
    modeFull: "Full",
    modeEasyDescription:
      "Plain tasks. One question at a time. Nothing destructive hidden.",
    modeFullDescription:
      "Dense tables, full configurators, and routing for every object.",
    systemMenu: "Workspace",
    signOut: "End session",
    signIn: "Connect",
    time: "Time",
    timeTrusted: "Clock trusted",
    timeUntrusted: "Clock untrusted",
    timeUntrustedHint:
      "Time cannot be verified. Timestamps may be wrong; do not rely on them.",
    theme: "Theme",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
    language: "Language",
    online: "Online",
    offline: "Offline",
    degraded: "Degraded",
    pending: "Pending",
    candidate: "Pending changes",
    apply: "Review & apply",
    discard: "Discard",
    review: "Review",
    showCli: "Show Duck CLI",
    showApi: "Show API",
    explainWhy: "Explain why",
    inspection: "Inspect",
    copyCli: "Copy Duck CLI",
    copyApi: "Copy API",
    closeInspector: "Close inspector",
    primary: "Primary action",
    question: "Question",
    columns: "Columns",
    filters: "Filters",
    runtime: "Runtime notes",
    diagnostics: "Diagnostics",
    easyHint: "Easy mode view",
    dependencies: "Dependencies",
    risk: "Risk",
    features: "Features",
    packages: "Packages",
    pinned: "Pinned",
    noResults: "Nothing matches that yet.",
    loading: "Loading…",
    empty: "Nothing here yet.",
    error: "We couldn't reach the system.",
    unknown: "Unknown",
    unavailable: "Unavailable",
    backToFull: "Back to Full mode",
    mode: "Mode",
    moreDetails: "More details",
    close: "Close",
    clear: "Clear",
    ref: "Ref",
    whatIsThis: "What is this?",
    easyModeHint: "In Easy mode",
  },

  health: {
    healthy: "System healthy",
    needsAttention: "Needs attention",
    degraded: "Degraded",
    unknown: "Waiting for status",
    overall: "Overall health",
    overview: "Observation",
    readiness: "Readiness",
    doctor: "Doctor",
    topology: "Topology",
    apiReachable: "API reachable",
    apiUnreachable: "API not reachable",
    openDoctor: "Open Doctor",
    activeIncidents: "Active incidents",
    pendingChanges: "Pending changes",
    none: "None",
    timeTrusted: "Clock trusted",
    timeUntrusted: "Clock untrusted",
  },

  actionbar: {
    noChanges: "No pending changes",
    changesWaiting: "changes waiting to apply",
    validateAndApply: "Validate & Apply",
    discardAll: "Discard all",
  },

  nav: {
    interfaces: "Interfaces",
    wan: "WAN",
    lan: "LAN / VLAN",
    routing: "Routing",
    firewall: "Firewall",
    tunnels: "Tunnels",
    dns: "DNS / DHCP",
    services: "Services",
    system: "System",
    logs: "Logs / Diagnostics",
    easy: "Easy",
  },

  overview: {
    title: "System overview",
    description:
      "What is failing now, what still works, and is anything waiting to apply.",
    issues: "Observed issues",
    noIssues: "No observation issues reported.",
    cpu: "CPU",
    memory: "Memory",
    storage: "Storage",
    uptime: "Uptime",
    interfaces: "Interfaces",
    routes: "Routes",
    dns: "DNS",
    system: "System",
    hostname: "Hostname",
    os: "OS",
    kernel: "Kernel",
    arch: "Architecture",
    observed: "Observed",
    notObserved: "Not observed",
    internetEvidence: "Internet evidence",
    noInternetEvidence: "No fresh Internet evidence yet.",
    cpuHint: "{cores} logical cores",
    interfacesHint: "{up} of {total} interfaces administratively up",
    uptimeHint: "Since the last boot",
    findingsTitle: "Active findings",
    identity: "Identity",
    architecture: "Architecture",
    mount: "Mount",
    filesystem: "Filesystem",
    used: "Used",
    available: "Available",
    noRoot: "No root filesystem observed.",
    shortcuts: "Quick navigation",
    topology: "Topology",
    candidate: "Candidate and Safe Apply",
    easy: "Switch to Easy mode",
  },

  candidate: {
    title: "Pending changes",
    empty: "No pending changes. The system matches the last applied revision.",
    description:
      "Candidate changes from any configurator or CLI run. Apply together or discard.",
    reviewTitle: "Review candidate",
    reviewDescription:
      "Confirm the change, blast radius, and confirmation window before applying.",
    noChanges: "Nothing to apply.",
    blastRadius: "Blast radius",
    confidence: "Confidence",
    owner: "Owner",
    operation: "Operation",
    before: "Before",
    after: "After",
    canary: "Canary",
    locks: "Locks",
    conflicts: "Conflicts",
    validate: "Validate",
    apply: "Apply",
    discard: "Discard",
    expires: "Expires",
    confirmWindow: "Confirm within",
    noConfirmSupport: "No commit-confirmed window — apply is irreversible.",
  },

  page: {
    easyHomeTitle: "Home",
    easyInternetTitle: "Internet",
    easyNetworksTitle: "Networks / Wi-Fi",
    easyDevicesTitle: "Devices / Access",
    easyPublishTitle: "Publish a service",
    easyCareTitle: "Care / Recovery",
  },

  inspector: {
    title: "Inspector",
    description:
      "Why this page exists, what changes look like, and how to do the same in Duck CLI or the API.",
    primaryAction: "Primary action",
    contextActions: "Context actions",
    cliLabel: "Duck CLI",
    apiLabel: "Engine / API",
    copyCli: "Copy CLI",
    copyApi: "Copy API",
    close: "Close",
    columnsHeader: "Columns",
    columnsHelp: "Pick which fields to show in the table.",
    dependenciesHeader: "Dependencies",
    noDeps: "No dependencies.",
  },

  common: {
    cancel: "Cancel",
    save: "Save",
    apply: "Apply",
    discard: "Discard",
    edit: "Edit",
    delete: "Delete",
    create: "Create",
    refresh: "Refresh",
    yes: "Yes",
    no: "No",
    none: "None",
    all: "All",
    more: "More",
    back: "Back",
    next: "Next",
    close: "Close",
    clear: "Clear",
    filter: "Filter…",
    loading: "Loading…",
    emptyRows: "Nothing here yet.",
    noResults: "No results.",
    selectPage: "Select a page to inspect.",
    observationStale: "Observation stale",
    observationStaleHint:
      "Last seen {seconds}s ago. Apply is disabled until the Engine reports a fresh state.",
    observationUntrusted: "Observation untrusted",
    pendingTxStale:
      "A pending transaction is reported but observation is stale. Do not assume success or failure until the dashboard shows a fresh Engine state. Open the Candidate page for the raw record.",
    rowsOf: "{visible} of {total}",
    pageOf: "{page} / {totalPages} · {rows} rows",
  },

  intent: {
    createInterface: "Create interface",
    editInterface: "Edit interface",
    deleteInterface: "Delete interface",
    createFirewallRule: "Create firewall rule",
    editFirewallRule: "Edit firewall rule",
    deleteFirewallRule: "Delete firewall rule",
    createWireguardServer: "Create WireGuard server",
    editWireguardServer: "Edit WireGuard server",
    deleteWireguardServer: "Delete WireGuard server",
    editWireguardPeer: "Edit WireGuard peer",
    createXrayServer: "Create Xray server",
    editXrayServer: "Edit Xray server",
    deleteXrayServer: "Delete Xray server",
    createQosClass: "Create QoS class",
    editQosClass: "Edit QoS class",
    deleteQosClass: "Delete QoS class",
    createDnsZone: "Create DNS zone",
    editDnsZone: "Edit DNS zone",
    deleteDnsZone: "Delete DNS zone",
    createDhcpScope: "Create DHCP scope",
    editDhcpScope: "Edit DHCP scope",
    deleteDhcpScope: "Delete DHCP scope",
    createAddress: "Create address",
    editAddress: "Edit address",
    deleteAddress: "Delete address",
    createRoute: "Create route",
    editRoute: "Edit route",
    deleteRoute: "Delete route",
    createRouteMap: "Create route map",
    editRouteMap: "Edit route map",
    deleteRouteMap: "Delete route map",
    createBridge: "Create bridge",
    editBridge: "Edit bridge",
    deleteBridge: "Delete bridge",
    createBond: "Create bond",
    editBond: "Edit bond",
    deleteBond: "Delete bond",
    createVlan: "Create VLAN",
    editVlan: "Edit VLAN",
    deleteVlan: "Delete VLAN",
    createBgpPeer: "Add BGP peer",
    editBgpPeer: "Edit BGP peer",
    deleteBgpPeer: "Remove BGP peer",
  },

  easy: {
    question5Title: "Five questions, one click each",
    question5Body:
      "Easy mode answers the only five things a non-expert operator needs to know. No configuration lives here — destructive changes stay in Full mode behind a confirmed Safe Apply.",
    home: {
      title: "Is everything working?",
      subtitle:
        "A one-screen view of the network. If something is wrong, you'll see it here first.",
      internetGood: "Internet is working",
      internetWarn: "Internet needs attention",
      internetBad: "Internet is down",
      internetUnknown: "Waiting for Internet signal",
    },
    internet: {
      title: "Internet",
      hint: "How this device reaches the rest of the world.",
      connected: "Internet is connected",
      degraded: "Internet is degraded",
      connection: "Connection",
      gateway: "Default gateway",
      interface: "Interface",
      dns: "DNS servers",
      searchDomains: "Search domains",
      uplinks: "Uplinks",
      noUplinks: "No uplink information exposed yet.",
      configure: "Configure Internet",
      configureHelp:
        "Set the WAN connection type, default gateway and DNS in Full mode. DuckOS never edits Internet settings without going through Safe Apply.",
      configureAction: "Open Internet settings",
    },
    wifi: {
      title: "Wi-Fi",
      hint: "The wireless networks this device is operating on.",
      detail: "Adapter status is not exposed by the public API yet.",
      configure: "Configure Wi-Fi",
    },
    networks: {
      title: "Networks",
      none: "No interfaces with addresses observed yet.",
      wifi: "Wi-Fi",
      wifiUnsupported: "Wi-Fi adapter status is not exposed yet.",
    },
    devices: {
      title: "Devices",
      hint: "Devices with an address this router can reach.",
      online: "online",
      onlineHint: "Counted from interfaces that are administratively up.",
      list: "Observed interfaces",
      none: "No devices observed yet.",
      view: "View Devices",
    },
    vpn: {
      title: "VPN",
      hint: "WireGuard / Xray tunnels configured on this device.",
      detail:
        "WireGuard and Xray servers are configured in the Tunnel section.",
      configure: "Configure VPN",
    },
    system: {
      title: "System",
      hint: "Overall device health and recent findings.",
      allGood: "All systems healthy",
      findings: "{count} findings to review",
      openDoctor: "Open Doctor",
      about: "About this device",
      hostname: "Hostname",
      uptime: "Uptime",
      environment: "Environment",
    },
    publish: {
      title: "Publish a service",
      hint: "Expose a LAN service to the Internet through DuckOS.",
      stepsTitle: "Steps to publish a service",
      step1: "1. Name and backend",
      step1Help: "Give the service a name and a LAN-side backend address.",
      step2: "2. Reachability",
      step2Help: "Make sure the backend address is reachable from DuckOS.",
      step3: "3. Ports and TLS",
      step3Help:
        "Open the right ports and attach a TLS certificate before exposing.",
      currently: "Currently published",
      noSites: "No sites have been published yet.",
      configureAction: "Open Sites",
    },
    care: {
      title: "Care and recovery",
      hint: "Doctor findings and the actions you can take when something is wrong.",
      allGood: "No findings",
      allGoodHint: "DuckOS sees no problems right now.",
      findings: "{count} findings",
      findingsHint: "Open Doctor for the full report.",
      findingsTitle: "Doctor findings",
      actionsTitle: "What you can do",
      actionsHelp:
        "Backups, updates and recovery live in Full mode. Destructive actions stay behind Safe Apply.",
      openDoctor: "Open Doctor",
      backup: "Backups",
      updates: "Updates",
    },
    tile: {
      open: "Open {title}",
      details: "Details",
    },
  },

  a11y: {
    openNavigation: "Open navigation",
    closeNavigation: "Close navigation",
    openInspector: "Open inspector",
    skipToMain: "Skip to main content",
    navPrimary: "Primary navigation",
    navSection: "Section navigation",
  },
};

/* Russian, German, Spanish — keep keys identical to the English bundle. */
const ru: Messages = {
  ...en,
  appTagline: "Ваша сеть. Без лишнего шума.",
  shell: {
    ...en.shell,
    searchPlaceholder: "Куда перейти? Введите имя, роль или раздел.",
    searchHint:
      "Поиск по рабочему пространству, не по живой сети. Enter — открыть страницу.",
    modeEasy: "Просто",
    modeFull: "Полный",
    modeEasyDescription:
      "Понятные задачи. Один вопрос за раз. Без скрытых действий.",
    modeFullDescription:
      "Плотные таблицы, полные конфигураторы и маршруты к каждому объекту.",
    signOut: "Завершить сеанс",
    signIn: "Подключиться",
    timeTrusted: "Время доверено",
    timeUntrusted: "Время не доверено",
    timeUntrustedHint:
      "Время не проверено. Метки времени могут быть неточны; не полагайтесь на них.",
    themeSystem: "Системная",
    themeLight: "Светлая",
    themeDark: "Тёмная",
    candidate: "Ожидающие изменения",
    apply: "Проверить и применить",
    discard: "Отменить",
    review: "Проверить",
    showCli: "Показать Duck CLI",
    showApi: "Показать API",
    explainWhy: "Объяснить",
    inspection: "Сведения",
    copyCli: "Скопировать Duck CLI",
    mode: "Режим",
    moreDetails: "Подробнее",
    close: "Закрыть",
    clear: "Очистить",
    ref: "ID",
    whatIsThis: "Что это?",
    easyModeHint: "В простом режиме",
    copyApi: "Скопировать API",
    noResults: "Ничего не найдено.",
    loading: "Загрузка…",
    empty: "Пока ничего.",
    error: "Не удалось связаться с системой.",
    unknown: "Неизвестно",
    unavailable: "Нет данных",
  },
  nav: {
    ...en.nav,
    interfaces: "Интерфейсы",
    wan: "WAN",
    lan: "LAN / VLAN",
    routing: "Маршрутизация",
    firewall: "Межсетевой экран",
    tunnels: "Туннели",
    dns: "DNS / DHCP",
    services: "Службы",
    system: "Система",
    logs: "Журналы / Диагностика",
    easy: "Просто",
  },
  health: {
    ...en.health,
    overall: "Общее состояние",
    activeIncidents: "Активные инциденты",
    pendingChanges: "Ожидающие изменения",
    healthy: "Система в порядке",
    needsAttention: "Нужно внимание",
    degraded: "Есть проблемы",
    unknown: "Ожидаем состояния",
  },
  actionbar: {
    ...en.actionbar,
    noChanges: "Нет ожидающих изменений",
    changesWaiting: "изменений ожидают применения",
    validateAndApply: "Проверить и применить",
    discardAll: "Отменить все",
  },
  overview: {
    ...en.overview,
    title: "Обзор системы",
    description:
      "Что сейчас не работает, что работает и есть ли ожидающие изменения.",
    issues: "Замеченные проблемы",
    noIssues: "Замечаний нет.",
    cpu: "ЦП",
    memory: "Память",
    storage: "Хранилище",
    uptime: "Аптайм",
    interfaces: "Интерфейсы",
    routes: "Маршруты",
    dns: "DNS",
    system: "Система",
    hostname: "Имя",
    os: "ОС",
    kernel: "Ядро",
    arch: "Архитектура",
    observed: "Наблюдается",
    notObserved: "Не наблюдается",
    internetEvidence: "Свидетельства Интернета",
    noInternetEvidence: "Свежих свидетельств Интернета пока нет.",
    cpuHint: "{cores} логических ядер",
    interfacesHint: "{up} из {total} интерфейсов в состоянии up",
    uptimeHint: "С момента последней загрузки",
    findingsTitle: "Активные находки",
    identity: "Идентификация",
    architecture: "Архитектура",
    mount: "Точка монтирования",
    filesystem: "Файловая система",
    used: "Использовано",
    available: "Доступно",
    noRoot: "Корневая файловая система не наблюдается.",
    shortcuts: "Быстрые переходы",
    topology: "Топология",
    candidate: "Кандидат и Safe Apply",
    easy: "Перейти в простой режим",
  },
  candidate: {
    ...en.candidate,
    title: "Ожидающие изменения",
    empty:
      "Изменений нет. Система соответствует последней применённой ревизии.",
  },
  page: {
    easyHomeTitle: "Главная",
    easyInternetTitle: "Интернет",
    easyNetworksTitle: "Сети / Wi-Fi",
    easyDevicesTitle: "Устройства / Доступ",
    easyPublishTitle: "Опубликовать службу",
    easyCareTitle: "Уход / Восстановление",
  },

  common: {
    cancel: "Отмена",
    save: "Сохранить",
    apply: "Применить",
    discard: "Отменить",
    edit: "Изменить",
    delete: "Удалить",
    create: "Создать",
    refresh: "Обновить",
    yes: "Да",
    no: "Нет",
    none: "Нет",
    all: "Все",
    more: "Ещё",
    back: "Назад",
    next: "Далее",
    close: "Закрыть",
    clear: "Очистить",
    filter: "Фильтр…",
    loading: "Загрузка…",
    emptyRows: "Пока ничего.",
    noResults: "Ничего не найдено.",
    selectPage: "Выберите страницу для просмотра.",
    observationStale: "Наблюдение устарело",
    observationStaleHint:
      "Последний раз видели {seconds} с назад. Применение отключено, пока Engine не сообщит свежее состояние.",
    observationUntrusted: "Наблюдение не доверено",
    pendingTxStale:
      "Заявлена ожидающая транзакция, но наблюдение устарело. Не считайте применение успешным или провальным, пока панель не покажет свежее состояние Engine. Откройте страницу кандидата для сырых данных.",
    rowsOf: "{visible} из {total}",
    pageOf: "{page} / {totalPages} · строк {rows}",
  },

  intent: {
    createInterface: "Создать интерфейс",
    editInterface: "Изменить интерфейс",
    deleteInterface: "Удалить интерфейс",
    createFirewallRule: "Создать правило межсетевого экрана",
    editFirewallRule: "Изменить правило межсетевого экрана",
    deleteFirewallRule: "Удалить правило межсетевого экрана",
    createWireguardServer: "Создать WireGuard-сервер",
    editWireguardServer: "Изменить WireGuard-сервер",
    deleteWireguardServer: "Удалить WireGuard-сервер",
    editWireguardPeer: "Изменить WireGuard-пира",
    createXrayServer: "Создать Xray-сервер",
    editXrayServer: "Изменить Xray-сервер",
    deleteXrayServer: "Удалить Xray-сервер",
    createQosClass: "Создать класс QoS",
    editQosClass: "Изменить класс QoS",
    deleteQosClass: "Удалить класс QoS",
    createDnsZone: "Создать DNS-зону",
    editDnsZone: "Изменить DNS-зону",
    deleteDnsZone: "Удалить DNS-зону",
    createDhcpScope: "Создать DHCP-область",
    editDhcpScope: "Изменить DHCP-область",
    deleteDhcpScope: "Удалить DHCP-область",
    createAddress: "Создать адрес",
    editAddress: "Изменить адрес",
    deleteAddress: "Удалить адрес",
    createRoute: "Создать маршрут",
    editRoute: "Изменить маршрут",
    deleteRoute: "Удалить маршрут",
    createRouteMap: "Создать route map",
    editRouteMap: "Изменить route map",
    deleteRouteMap: "Удалить route map",
    createBridge: "Создать мост",
    editBridge: "Изменить мост",
    deleteBridge: "Удалить мост",
    createBond: "Создать bond",
    editBond: "Изменить bond",
    deleteBond: "Удалить bond",
    createVlan: "Создать VLAN",
    editVlan: "Изменить VLAN",
    deleteVlan: "Удалить VLAN",
    createBgpPeer: "Добавить BGP-пира",
    editBgpPeer: "Изменить BGP-пира",
    deleteBgpPeer: "Удалить BGP-пира",
  },

  easy: {
    question5Title: "Пять вопросов — по одному клику",
    question5Body:
      "Простой режим отвечает на пять вещей, которые нужны обычному администратору. Здесь нет настроек — деструктивные действия остаются в полном режиме за подтверждённой Safe Apply.",
    home: {
      title: "Всё ли работает?",
      subtitle:
        "Один экран о состоянии сети. Если что-то не так — вы увидите это здесь первым.",
      internetGood: "Интернет работает",
      internetWarn: "Интернет требует внимания",
      internetBad: "Интернет недоступен",
      internetUnknown: "Ждём сигнала об Интернете",
    },
    internet: {
      title: "Интернет",
      hint: "Как это устройство связывается с внешним миром.",
      connected: "Интернет подключён",
      degraded: "Интернет работает с перебоями",
      connection: "Соединение",
      gateway: "Шлюз по умолчанию",
      interface: "Интерфейс",
      dns: "DNS-серверы",
      searchDomains: "Домены поиска",
      uplinks: "Каналы",
      noUplinks: "Информация о каналах пока недоступна.",
      configure: "Настроить интернет",
      configureHelp:
        "Тип WAN-соединения, шлюз и DNS задаются в полном режиме. DuckOS не меняет их без подтверждения через Safe Apply.",
      configureAction: "Открыть настройки Интернета",
    },
    wifi: {
      title: "Wi-Fi",
      hint: "Беспроводные сети, обслуживаемые этим устройством.",
      detail: "Состояние адаптера пока не доступно через публичный API.",
      configure: "Настроить Wi-Fi",
    },
    networks: {
      title: "Сети",
      none: "Интерфейсов с адресами пока не видно.",
      wifi: "Wi-Fi",
      wifiUnsupported: "Состояние Wi-Fi адаптера пока недоступно.",
    },
    devices: {
      title: "Устройства",
      hint: "Устройства с адресом, до которого маршрутизатор может достучаться.",
      online: "в сети",
      onlineHint: "Считаются только интерфейсы в состоянии up.",
      list: "Наблюдаемые интерфейсы",
      none: "Устройств пока не видно.",
      view: "Открыть устройства",
    },
    vpn: {
      title: "VPN",
      hint: "WireGuard / Xray тоннели, настроенные на устройстве.",
      detail: "Серверы WireGuard и Xray настраиваются в разделе Тоннели.",
      configure: "Настроить VPN",
    },
    system: {
      title: "Система",
      hint: "Общее состояние устройства и свежие находки.",
      allGood: "Всё в порядке",
      findings: "Найдено {count} замечаний",
      openDoctor: "Открыть Doctor",
      about: "Об устройстве",
      hostname: "Имя хоста",
      uptime: "Время работы",
      environment: "Окружение",
    },
    publish: {
      title: "Опубликовать службу",
      hint: "Пробросить LAN-службу в Интернет через DuckOS.",
      stepsTitle: "Шаги",
      step1: "1. Имя и backend",
      step1Help: "Дайте службе имя и LAN-адрес backend.",
      step2: "2. Достижимость",
      step2Help: "Убедитесь, что backend достижим с DuckOS.",
      step3: "3. Порты и TLS",
      step3Help: "Откройте нужные порты и прикрепите TLS-сертификат.",
      currently: "Опубликованные",
      noSites: "Сайты ещё не публиковались.",
      configureAction: "Открыть Sites",
    },
    care: {
      title: "Обслуживание и восстановление",
      hint: "Находки Doctor и действия, которые можно предпринять.",
      allGood: "Нет замечаний",
      allGoodHint: "DuckOS сейчас не видит проблем.",
      findings: "{count} замечаний",
      findingsHint: "Откройте Doctor для полного отчёта.",
      findingsTitle: "Находки Doctor",
      actionsTitle: "Что можно сделать",
      actionsHelp:
        "Резервные копии, обновления и восстановление — в полном режиме.",
      openDoctor: "Открыть Doctor",
      backup: "Резервные копии",
      updates: "Обновления",
    },
    tile: {
      open: "Открыть {title}",
      details: "Подробнее",
    },
  },
};

const de: Messages = {
  ...en,
  appTagline: "Ihr Netzwerk. Klar.",
  shell: {
    ...en.shell,
    searchPlaceholder: "Wohin? Name, Rolle oder Bereich eingeben.",
    searchHint:
      "Durchsucht das Workspace, nicht das Live-Netz. Enter öffnet die Seite.",
    modeEasy: "Einfach",
    modeFull: "Voll",
    modeEasyDescription: "Klare Aufgaben. Eine Frage nach der anderen.",
    modeFullDescription:
      "Dichte Tabellen, volle Konfiguratoren, Wege zu jedem Objekt.",
    signOut: "Sitzung beenden",
    signIn: "Verbinden",
    timeTrusted: "Zeit vertrauenswürdig",
    timeUntrusted: "Zeit nicht vertrauenswürdig",
    themeSystem: "System",
    themeLight: "Hell",
    themeDark: "Dunkel",
    candidate: "Ausstehende Änderungen",
    apply: "Prüfen & anwenden",
    discard: "Verwerfen",
    review: "Prüfen",
    showCli: "Duck CLI zeigen",
    showApi: "API zeigen",
    explainWhy: "Warum?",
    inspection: "Details",
    copyCli: "Duck CLI kopieren",
    copyApi: "API kopieren",
    noResults: "Keine Treffer.",
    loading: "Lädt…",
    empty: "Noch nichts da.",
    error: "System nicht erreichbar.",
    unknown: "Unbekannt",
    unavailable: "Nicht verfügbar",
  },
  nav: {
    ...en.nav,
    interfaces: "Schnittstellen",
    wan: "WAN",
    lan: "LAN / VLAN",
    routing: "Routing",
    firewall: "Firewall",
    tunnels: "Tunnel",
    dns: "DNS / DHCP",
    services: "Dienste",
    system: "System",
    logs: "Protokolle / Diagnose",
    easy: "Einfach",
  },
  health: {
    ...en.health,
    overall: "Gesamtzustand",
    activeIncidents: "Aktive Vorfälle",
    pendingChanges: "Ausstehende Änderungen",
  },
  overview: {
    ...en.overview,
    title: "Systemübersicht",
    description: "Was fällt aus, was läuft, und ist eine Änderung offen?",
    issues: "Beobachtete Probleme",
    noIssues: "Keine Probleme gemeldet.",
    cpu: "CPU",
    memory: "Speicher",
    storage: "Speicherplatz",
    uptime: "Laufzeit",
    interfaces: "Schnittstellen",
    routes: "Routen",
    dns: "DNS",
    system: "System",
    hostname: "Hostname",
    os: "OS",
    kernel: "Kernel",
    arch: "Architektur",
    observed: "Beobachtet",
    notObserved: "Nicht beobachtet",
    internetEvidence: "Internet-Beleg",
    noInternetEvidence: "Noch kein frischer Internet-Beleg.",
    cpuHint: "{cores} logische Kerne",
    interfacesHint: "{up} von {total} Schnittstellen administrativ up",
    uptimeHint: "Seit dem letzten Boot",
    findingsTitle: "Aktive Befunde",
    identity: "Identität",
    architecture: "Architektur",
    mount: "Mount",
    filesystem: "Dateisystem",
    used: "Belegt",
    available: "Verfügbar",
    noRoot: "Kein Root-Dateisystem beobachtet.",
    shortcuts: "Schnellzugriff",
    topology: "Topologie",
    candidate: "Kandidat und Safe Apply",
    easy: "Zum Einfach-Modus wechseln",
  },
  candidate: {
    ...en.candidate,
    title: "Ausstehende Änderungen",
    empty: "Keine Änderungen offen. System entspricht der letzten Revision.",
  },
  page: {
    easyHomeTitle: "Start",
    easyInternetTitle: "Internet",
    easyNetworksTitle: "Netze / WLAN",
    easyDevicesTitle: "Geräte / Zugriff",
    easyPublishTitle: "Dienst veröffentlichen",
    easyCareTitle: "Pflege / Wiederherstellung",
  },

  common: {
    cancel: "Abbrechen",
    save: "Speichern",
    apply: "Anwenden",
    discard: "Verwerfen",
    edit: "Bearbeiten",
    delete: "Löschen",
    create: "Erstellen",
    refresh: "Neu laden",
    yes: "Ja",
    no: "Nein",
    none: "Keine",
    all: "Alle",
    more: "Mehr",
    back: "Zurück",
    next: "Weiter",
    close: "Schließen",
    clear: "Leeren",
    filter: "Filtern…",
    loading: "Lädt…",
    emptyRows: "Noch nichts da.",
    noResults: "Keine Treffer.",
    selectPage: "Seite zum Prüfen auswählen.",
    observationStale: "Beobachtung veraltet",
    observationStaleHint:
      "Zuletzt vor {seconds} s gesehen. Anwenden ist deaktiviert, bis die Engine einen frischen Zustand meldet.",
    observationUntrusted: "Beobachtung nicht vertrauenswürdig",
    pendingTxStale:
      "Eine ausstehende Transaktion wird gemeldet, aber die Beobachtung ist veraltet. Erfolg oder Fehler erst annehmen, wenn das Dashboard einen frischen Engine-Zustand zeigt. Rohdaten auf der Kandidaten-Seite öffnen.",
    rowsOf: "{visible} von {total}",
    pageOf: "{page} / {totalPages} · {rows} Zeilen",
  },

  intent: {
    createInterface: "Schnittstelle erstellen",
    editInterface: "Schnittstelle bearbeiten",
    deleteInterface: "Schnittstelle löschen",
    createFirewallRule: "Firewall-Regel erstellen",
    editFirewallRule: "Firewall-Regel bearbeiten",
    deleteFirewallRule: "Firewall-Regel löschen",
    createWireguardServer: "WireGuard-Server erstellen",
    editWireguardServer: "WireGuard-Server bearbeiten",
    deleteWireguardServer: "WireGuard-Server löschen",
    editWireguardPeer: "WireGuard-Peer bearbeiten",
    createXrayServer: "Xray-Server erstellen",
    editXrayServer: "Xray-Server bearbeiten",
    deleteXrayServer: "Xray-Server löschen",
    createQosClass: "QoS-Klasse erstellen",
    editQosClass: "QoS-Klasse bearbeiten",
    deleteQosClass: "QoS-Klasse löschen",
    createDnsZone: "DNS-Zone erstellen",
    editDnsZone: "DNS-Zone bearbeiten",
    deleteDnsZone: "DNS-Zone löschen",
    createDhcpScope: "DHCP-Bereich erstellen",
    editDhcpScope: "DHCP-Bereich bearbeiten",
    deleteDhcpScope: "DHCP-Bereich löschen",
    createAddress: "Adresse erstellen",
    editAddress: "Adresse bearbeiten",
    deleteAddress: "Adresse löschen",
    createRoute: "Route erstellen",
    editRoute: "Route bearbeiten",
    deleteRoute: "Route löschen",
    createRouteMap: "Route-Map erstellen",
    editRouteMap: "Route-Map bearbeiten",
    deleteRouteMap: "Route-Map löschen",
    createBridge: "Bridge erstellen",
    editBridge: "Bridge bearbeiten",
    deleteBridge: "Bridge löschen",
    createBond: "Bond erstellen",
    editBond: "Bond bearbeiten",
    deleteBond: "Bond löschen",
    createVlan: "VLAN erstellen",
    editVlan: "VLAN bearbeiten",
    deleteVlan: "VLAN löschen",
    createBgpPeer: "BGP-Peer hinzufügen",
    editBgpPeer: "BGP-Peer bearbeiten",
    deleteBgpPeer: "BGP-Peer entfernen",
  },

  easy: {
    question5Title: "Fünf Fragen, ein Klick pro Antwort",
    question5Body:
      "Der Einfach-Modus beantwortet die fünf Dinge, die eine nicht-technische Bedienperson braucht. Hier gibt es keine Konfiguration — destruktive Änderungen bleiben im Voll-Modus hinter einem bestätigten Safe Apply.",
    home: {
      title: "Funktioniert alles?",
      subtitle:
        "Ein Bildschirm zum Netzwerkzustand. Wenn etwas nicht stimmt, sehen Sie es hier zuerst.",
      internetGood: "Internet funktioniert",
      internetWarn: "Internet braucht Aufmerksamkeit",
      internetBad: "Internet ist ausgefallen",
      internetUnknown: "Warte auf Internet-Signal",
    },
    internet: {
      title: "Internet",
      hint: "Wie dieses Gerät die Außenwelt erreicht.",
      connected: "Internet ist verbunden",
      degraded: "Internet ist eingeschränkt",
      connection: "Verbindung",
      gateway: "Standard-Gateway",
      interface: "Schnittstelle",
      dns: "DNS-Server",
      searchDomains: "Suchdomänen",
      uplinks: "Uplinks",
      noUplinks: "Noch keine Uplink-Informationen verfügbar.",
      configure: "Internet einrichten",
      configureHelp:
        "WAN-Typ, Standard-Gateway und DNS werden im Voll-Modus gesetzt. DuckOS ändert das nie ohne Safe Apply.",
      configureAction: "Interneteinstellungen öffnen",
    },
    wifi: {
      title: "WLAN",
      hint: "Drahtlosnetze, die dieses Gerät bedient.",
      detail:
        "Adapterstatus ist noch nicht über die öffentliche API verfügbar.",
      configure: "WLAN einrichten",
    },
    networks: {
      title: "Netze",
      none: "Noch keine Schnittstellen mit Adressen beobachtet.",
      wifi: "WLAN",
      wifiUnsupported: "WLAN-Adapterstatus noch nicht verfügbar.",
    },
    devices: {
      title: "Geräte",
      hint: "Geräte mit einer Adresse, die dieser Router sehen kann.",
      online: "online",
      onlineHint: "Nur Schnittstellen im Zustand up werden gezählt.",
      list: "Beobachtete Schnittstellen",
      none: "Noch keine Geräte beobachtet.",
      view: "Geräte öffnen",
    },
    vpn: {
      title: "VPN",
      hint: "WireGuard- / Xray-Tunnel auf diesem Gerät.",
      detail:
        "WireGuard- und Xray-Server werden im Tunnel-Bereich konfiguriert.",
      configure: "VPN einrichten",
    },
    system: {
      title: "System",
      hint: "Gerätezustand und letzte Befunde.",
      allGood: "Alles gesund",
      findings: "{count} Befunde zu prüfen",
      openDoctor: "Doctor öffnen",
      about: "Über dieses Gerät",
      hostname: "Hostname",
      uptime: "Laufzeit",
      environment: "Umgebung",
    },
    publish: {
      title: "Dienst veröffentlichen",
      hint: "Einen LAN-Dienst über DuckOS ins Internet freigeben.",
      stepsTitle: "Schritte",
      step1: "1. Name und Backend",
      step1Help:
        "Geben Sie dem Dienst einen Namen und eine LAN-Backend-Adresse.",
      step2: "2. Erreichbarkeit",
      step2Help:
        "Sicherstellen, dass die Backend-Adresse von DuckOS erreichbar ist.",
      step3: "3. Ports und TLS",
      step3Help: "Passende Ports öffnen und TLS-Zertifikat zuweisen.",
      currently: "Aktuell veröffentlicht",
      noSites: "Es wurden noch keine Sites veröffentlicht.",
      configureAction: "Sites öffnen",
    },
    care: {
      title: "Pflege und Wiederherstellung",
      hint: "Doctor-Befunde und mögliche Aktionen bei Problemen.",
      allGood: "Keine Befunde",
      allGoodHint: "DuckOS sieht aktuell keine Probleme.",
      findings: "{count} Befunde",
      findingsHint: "Doctor für den vollständigen Bericht öffnen.",
      findingsTitle: "Doctor-Befunde",
      actionsTitle: "Was Sie tun können",
      actionsHelp:
        "Backups, Updates und Wiederherstellung liegen im Voll-Modus.",
      openDoctor: "Doctor öffnen",
      backup: "Backups",
      updates: "Updates",
    },
    tile: {
      open: "{title} öffnen",
      details: "Details",
    },
  },
};

const es: Messages = {
  ...en,
  appTagline: "Tu red. Clara.",
  shell: {
    ...en.shell,
    searchPlaceholder: "¿A dónde? Escribe nombre, rol o sección.",
    searchHint:
      "Busca en el espacio de trabajo, no en la red en vivo. Enter abre la página.",
    modeEasy: "Fácil",
    modeFull: "Completo",
    modeEasyDescription: "Tareas claras. Una pregunta a la vez.",
    modeFullDescription:
      "Tablas densas, configuradores completos, rutas a cada objeto.",
    signOut: "Cerrar sesión",
    signIn: "Conectar",
    timeTrusted: "Hora confiable",
    timeUntrusted: "Hora no confiable",
    themeSystem: "Sistema",
    themeLight: "Claro",
    themeDark: "Oscuro",
    candidate: "Cambios pendientes",
    apply: "Revisar y aplicar",
    discard: "Descartar",
    review: "Revisar",
    showCli: "Mostrar Duck CLI",
    showApi: "Mostrar API",
    explainWhy: "¿Por qué?",
    inspection: "Detalle",
    copyCli: "Copiar Duck CLI",
    copyApi: "Copiar API",
    noResults: "Sin coincidencias.",
    loading: "Cargando…",
    empty: "Nada todavía.",
    error: "No se pudo contactar al sistema.",
    unknown: "Desconocido",
    unavailable: "No disponible",
  },
  nav: {
    ...en.nav,
    interfaces: "Interfaces",
    wan: "WAN",
    lan: "LAN / VLAN",
    routing: "Enrutamiento",
    firewall: "Cortafuegos",
    tunnels: "Túneles",
    dns: "DNS / DHCP",
    services: "Servicios",
    system: "Sistema",
    logs: "Registros / Diagnóstico",
    easy: "Fácil",
  },
  health: {
    ...en.health,
    overall: "Estado general",
    activeIncidents: "Incidentes activos",
    pendingChanges: "Cambios pendientes",
  },
  overview: {
    ...en.overview,
    title: "Resumen del sistema",
    description: "¿Qué falla ahora, qué sigue y hay un cambio esperando?",
    issues: "Problemas observados",
    noIssues: "Sin problemas reportados.",
    cpu: "CPU",
    memory: "Memoria",
    storage: "Almacenamiento",
    uptime: "Actividad",
    interfaces: "Interfaces",
    routes: "Rutas",
    dns: "DNS",
    system: "Sistema",
    hostname: "Nombre",
    os: "SO",
    kernel: "Núcleo",
    arch: "Arquitectura",
    observed: "Observado",
    notObserved: "No observado",
    internetEvidence: "Evidencia de Internet",
    noInternetEvidence: "Sin evidencia reciente de Internet.",
    cpuHint: "{cores} núcleos lógicos",
    interfacesHint: "{up} de {total} interfaces administrativamente up",
    uptimeHint: "Desde el último arranque",
    findingsTitle: "Hallazgos activos",
    identity: "Identidad",
    architecture: "Arquitectura",
    mount: "Punto de montaje",
    filesystem: "Sistema de archivos",
    used: "Usado",
    available: "Disponible",
    noRoot: "Sin sistema de archivos raíz observado.",
    shortcuts: "Accesos rápidos",
    topology: "Topología",
    candidate: "Candidate y Safe Apply",
    easy: "Cambiar al modo Fácil",
  },
  candidate: {
    ...en.candidate,
    title: "Cambios pendientes",
    empty:
      "Sin cambios pendientes. El sistema coincide con la última revisión.",
  },
  page: {
    easyHomeTitle: "Inicio",
    easyInternetTitle: "Internet",
    easyNetworksTitle: "Redes / Wi-Fi",
    easyDevicesTitle: "Dispositivos / Acceso",
    easyPublishTitle: "Publicar un servicio",
    easyCareTitle: "Cuidado / Recuperación",
  },

  common: {
    cancel: "Cancelar",
    save: "Guardar",
    apply: "Aplicar",
    discard: "Descartar",
    edit: "Editar",
    delete: "Eliminar",
    create: "Crear",
    refresh: "Recargar",
    yes: "Sí",
    no: "No",
    none: "Ninguno",
    all: "Todos",
    more: "Más",
    back: "Atrás",
    next: "Siguiente",
    close: "Cerrar",
    clear: "Limpiar",
    filter: "Filtrar…",
    loading: "Cargando…",
    emptyRows: "Nada todavía.",
    noResults: "Sin resultados.",
    selectPage: "Selecciona una página para inspeccionar.",
    observationStale: "Observación anticuada",
    observationStaleHint:
      "Visto por última vez hace {seconds} s. Aplicar está deshabilitado hasta que el Engine reporte un estado nuevo.",
    observationUntrusted: "Observación no confiable",
    pendingTxStale:
      "Se reporta una transacción pendiente pero la observación está anticuada. No asumas éxito ni fallo hasta que el panel muestre un estado nuevo del Engine. Abre la página Candidate para ver el registro crudo.",
    rowsOf: "{visible} de {total}",
    pageOf: "{page} / {totalPages} · {rows} filas",
  },

  intent: {
    createInterface: "Crear interfaz",
    editInterface: "Editar interfaz",
    deleteInterface: "Eliminar interfaz",
    createFirewallRule: "Crear regla de cortafuegos",
    editFirewallRule: "Editar regla de cortafuegos",
    deleteFirewallRule: "Eliminar regla de cortafuegos",
    createWireguardServer: "Crear servidor WireGuard",
    editWireguardServer: "Editar servidor WireGuard",
    deleteWireguardServer: "Eliminar servidor WireGuard",
    editWireguardPeer: "Editar peer WireGuard",
    createXrayServer: "Crear servidor Xray",
    editXrayServer: "Editar servidor Xray",
    deleteXrayServer: "Eliminar servidor Xray",
    createQosClass: "Crear clase QoS",
    editQosClass: "Editar clase QoS",
    deleteQosClass: "Eliminar clase QoS",
    createDnsZone: "Crear zona DNS",
    editDnsZone: "Editar zona DNS",
    deleteDnsZone: "Eliminar zona DNS",
    createDhcpScope: "Crear ámbito DHCP",
    editDhcpScope: "Editar ámbito DHCP",
    deleteDhcpScope: "Eliminar ámbito DHCP",
    createAddress: "Crear dirección",
    editAddress: "Editar dirección",
    deleteAddress: "Eliminar dirección",
    createRoute: "Crear ruta",
    editRoute: "Editar ruta",
    deleteRoute: "Eliminar ruta",
    createRouteMap: "Crear route-map",
    editRouteMap: "Editar route-map",
    deleteRouteMap: "Eliminar route-map",
    createBridge: "Crear bridge",
    editBridge: "Editar bridge",
    deleteBridge: "Eliminar bridge",
    createBond: "Crear bond",
    editBond: "Editar bond",
    deleteBond: "Eliminar bond",
    createVlan: "Crear VLAN",
    editVlan: "Editar VLAN",
    deleteVlan: "Eliminar VLAN",
    createBgpPeer: "Añadir peer BGP",
    editBgpPeer: "Editar peer BGP",
    deleteBgpPeer: "Eliminar peer BGP",
  },

  easy: {
    question5Title: "Cinco preguntas, un clic cada una",
    question5Body:
      "El modo Fácil responde las cinco cosas que necesita un operador no experto. Aquí no hay configuración — los cambios destructivos quedan en el modo Completo detrás de un Safe Apply confirmado.",
    home: {
      title: "¿Funciona todo?",
      subtitle:
        "Una vista de un solo panel del estado de la red. Si algo falla, lo verá aquí primero.",
      internetGood: "Internet funciona",
      internetWarn: "Internet necesita atención",
      internetBad: "Internet está caído",
      internetUnknown: "Esperando señal de Internet",
    },
    internet: {
      title: "Internet",
      hint: "Cómo este dispositivo llega al resto del mundo.",
      connected: "Internet conectado",
      degraded: "Internet degradado",
      connection: "Conexión",
      gateway: "Puerta de enlace",
      interface: "Interfaz",
      dns: "Servidores DNS",
      searchDomains: "Dominios de búsqueda",
      uplinks: "Uplinks",
      noUplinks: "Aún no hay información de uplink.",
      configure: "Configurar Internet",
      configureHelp:
        "Tipo de WAN, puerta de enlace y DNS se definen en el modo Completo. DuckOS nunca los cambia sin Safe Apply.",
      configureAction: "Abrir configuración de Internet",
    },
    wifi: {
      title: "Wi-Fi",
      hint: "Redes inalámbricas que opera este dispositivo.",
      detail:
        "El estado del adaptador aún no está disponible en la API pública.",
      configure: "Configurar Wi-Fi",
    },
    networks: {
      title: "Redes",
      none: "Aún no se ven interfaces con direcciones.",
      wifi: "Wi-Fi",
      wifiUnsupported: "El estado del adaptador Wi-Fi aún no está disponible.",
    },
    devices: {
      title: "Dispositivos",
      hint: "Dispositivos con una dirección que este router puede ver.",
      online: "en línea",
      onlineHint: "Se cuentan solo las interfaces en estado up.",
      list: "Interfaces observadas",
      none: "Aún no se ven dispositivos.",
      view: "Abrir dispositivos",
    },
    vpn: {
      title: "VPN",
      hint: "Túneles WireGuard / Xray configurados en este dispositivo.",
      detail:
        "Los servidores WireGuard y Xray se configuran en la sección Túneles.",
      configure: "Configurar VPN",
    },
    system: {
      title: "Sistema",
      hint: "Estado del dispositivo y hallazgos recientes.",
      allGood: "Todo sano",
      findings: "{count} hallazgos para revisar",
      openDoctor: "Abrir Doctor",
      about: "Sobre este dispositivo",
      hostname: "Hostname",
      uptime: "Tiempo activo",
      environment: "Entorno",
    },
    publish: {
      title: "Publicar un servicio",
      hint: "Exponer un servicio LAN a Internet a través de DuckOS.",
      stepsTitle: "Pasos",
      step1: "1. Nombre y backend",
      step1Help: "Asigna al servicio un nombre y una dirección backend LAN.",
      step2: "2. Alcanzabilidad",
      step2Help:
        "Asegúrate de que la dirección backend sea alcanzable desde DuckOS.",
      step3: "3. Puertos y TLS",
      step3Help: "Abre los puertos adecuados y asigna un certificado TLS.",
      currently: "Publicado actualmente",
      noSites: "Aún no se han publicado sitios.",
      configureAction: "Abrir Sites",
    },
    care: {
      title: "Cuidado y recuperación",
      hint: "Hallazgos de Doctor y acciones posibles cuando algo falla.",
      allGood: "Sin hallazgos",
      allGoodHint: "DuckOS no detecta problemas ahora mismo.",
      findings: "{count} hallazgos",
      findingsHint: "Abrir Doctor para el informe completo.",
      findingsTitle: "Hallazgos de Doctor",
      actionsTitle: "Qué puedes hacer",
      actionsHelp:
        "Respaldos, actualizaciones y recuperación están en el modo Completo.",
      openDoctor: "Abrir Doctor",
      backup: "Respaldos",
      updates: "Actualizaciones",
    },
    tile: {
      open: "Abrir {title}",
      details: "Detalles",
    },
  },
};

export const messages: Record<Locale, Messages> = { en, ru, de, es };

/**
 * Type-safe message accessor with English fallback. UI code calls
 * `t(messages[locale], "shell.apply")`; a missing key shows the English
 * value with a development console warning, never a blank.
 *
 * The accessor walks dot-separated paths so nested keys such as
 * `shell.language` or `candidate.title` resolve through the bundle.
 * The original implementation only checked top-level `path in bundle`,
 * which meant every nested-key call surfaced the raw key text (for
 * example `aria-label="shell.language"`). See UD-001 in
 * docs/ui/V1_LIVE_QUALIFICATION_STATUS.md.
 */
export function t(bundle: Messages, path: keyof Messages | string): string {
  const segments = String(path).split(".");
  let cursor: unknown = bundle;
  for (const segment of segments) {
    if (
      cursor &&
      typeof cursor === "object" &&
      segment in (cursor as Record<string, unknown>)
    ) {
      cursor = (cursor as Record<string, unknown>)[segment];
    } else {
      cursor = undefined;
      break;
    }
  }
  if (typeof cursor === "string" && cursor.length > 0) {
    return cursor;
  }
  let fallback: unknown = en;
  for (const segment of segments) {
    if (
      fallback &&
      typeof fallback === "object" &&
      segment in (fallback as Record<string, unknown>)
    ) {
      fallback = (fallback as Record<string, unknown>)[segment];
    } else {
      fallback = undefined;
      break;
    }
  }
  if (typeof fallback === "string" && fallback.length > 0) {
    return fallback;
  }
  return String(path);
}

export function detectLocale(): Locale {
  const isValid = (s: string | null): s is Locale =>
    s === "ru" || s === "de" || s === "es" || s === "en";
  // 1. The active document sets `data-locale` once the AppShell mounts.
  //    This is the source of truth — it reflects the user's pick in the
  //    topbar, including override via localStorage.
  if (typeof document !== "undefined") {
    const htmlLocale = document.documentElement.getAttribute("data-locale");
    if (isValid(htmlLocale)) return htmlLocale;
    const shellLocale =
      document.querySelector(".duck-shell")?.getAttribute("data-locale") ??
      null;
    if (isValid(shellLocale)) return shellLocale;
  }
  // 2. Fall back to the browser's preferred language.
  if (typeof navigator === "undefined") return "en";
  const candidate = navigator.language?.toLowerCase().split(/[-_]/)[0];
  if (isValid(candidate)) return candidate;
  return "en";
}
