export default {
  language: {
    label: 'Язык',
    en: 'EN',
    ru: 'РУ'
  },
  brand: {
    name: 'LlamaLens',
    sub: 'мониторинг llama-server в реальном времени'
  },
  app: {
    title: 'LlamaLens · мониторинг llama-server в реальном времени'
  },
  portal: {
    noHosts: 'Нет подключённых хостов',
    waiting: 'Ожидание хостов...'
  },
  router: {
    portal: 'Портал',
    host: 'Хост'
  },
  theme: {
    labels: {
      aurora: 'Aurora',
      terminal: 'Terminal',
      default: 'Default'
    }
  },
  hostStatus: {
    allHosts: 'Все хосты',
    nA: 'Н/Д',
    offline: 'Не в сети',
    connected: 'Подключён',
    disconnected: 'Отключён'
  },
  speed: {
    label: 'Скорость',
    noActiveSlot: 'Нет активных слотов',
    noModelsLoaded: 'Модели не загружены'
  },
  hostCard: {
    model: 'Модель',
    params: 'Параметры',
    speed: 'Скорость',
    gpu: 'GPU',
    vram: 'VRAM',
    temp: 'Темп.',
    power: 'Мощность',
    fan: 'Вентилятор',
    cpu: 'CPU',
    mem: 'ОЗУ',
    nA: 'Н/Д',
    noModelLoaded: 'Модель не загружена',
    noData: 'Нет данных'
  },
  gpu: {
    utilization: 'Загрузка GPU',
    memory: 'Память GPU',
    temperature: 'Температура',
    power: 'Мощность',
    fanSpeed: 'Обороты вентилятора',
    gpu: 'GPU',
    percent: '%',
    celsius: '°C',
    watts: 'Вт'
  },
  cpu: {
    usage: 'Загрузка CPU',
    cores: 'Ядра',
    loadAverage: 'Средняя нагрузка',
    frequency: 'Частота',
    cpu: 'CPU',
    percent: '%',
    cores: 'ядер',
    load: 'нагрузка',
    mhz: 'МГц'
  },
  memory: {
    usage: 'Использование ОЗУ',
    swap: 'Swap',
    cache: 'Кэш',
    mem: 'ОЗУ',
    percent: '%',
    gb: 'ГБ'
  },
  network: {
    label: 'Сеть',
    interface: 'Интерфейс',
    rx: 'RX',
    tx: 'TX',
    mbps: 'Мбит/с'
  },
  disk: {
    label: 'Диск',
    mount: 'Монтирование',
    read: 'Чтение',
    write: 'Запись',
    mbps: 'МБ/с'
  },
  modelInfo: {
    path: 'Путь к модели',
    parameters: 'Параметры',
    embedding: 'Встраивание',
    vocab: 'Словарь',
    nA: 'Н/Д'
  },
  llamaState: {
    phase: 'Фаза',
    taskId: 'ID задачи',
    promptProcessing: 'Обработка запроса',
    generation: 'Генерация',
    idle: 'Ожидание',
    processing: 'Обработка',
    generating: 'Генерация',
    tokens: 'Токены',
    prompt: 'Запрос',
    eval: 'Вычисление',
    total: 'Всего',
    nA: 'Н/Д'
  },
  llamaProcess: {
    process: 'Процесс',
    pid: 'PID',
    cpu: 'CPU%',
    rss: 'RSS',
    vsz: 'VSZ',
    service: 'Сервис',
    nA: 'Н/Д'
  },
  slotTable: {
    slot: 'Слот',
    id: 'ID',
    context: 'Контекст',
    decoded: 'Декодировано',
    speed: 'Скорость',
    nA: 'Н/Д'
  },
  topProcessTable: {
    pid: 'PID',
    cpu: 'CPU%',
    mem: 'MEM%',
    rss: 'RSS',
    command: 'ПРОЦЕСС'
  },
  eventFeed: {
    title: 'Поток событий',
    noEvents: 'Событий пока нет',
    newEvent: 'Новое событие'
  },
  themeSwitcher: {
    label: 'Тема'
  },
  terminal: {
    prompt: '$ ',
    title: 'Терминал',
    connected: 'Подключён',
    disconnected: 'Отключён'
  },
  hostDetail: {
    title: 'Детали хоста',
    systemResources: 'Системные ресурсы',
    modelInfo: 'Информация о модели',
    llamaState: 'Состояние Llama',
    processList: 'Список процессов',
    events: 'События',
    gpu: 'GPU',
    cpu: 'CPU',
    memory: 'ОЗУ',
    network: 'Сеть',
    disk: 'Диск'
  },
  topBar: {
    back: 'Назад',
    speed: 'Скорость',
    context: 'Контекст',
    memory: 'ОЗУ',
    llama_offline: 'LLaMA офлайн',
    ssh_disconnected: 'SSH отключён',
    online: 'В сети',
    realtime: 'Реальное время',
    paused: 'Пауза',
    ws_disconnected: 'WS отключён',
    util_temp_power_mem: 'Загр / Темп / Мощн / ОЗУ / Всего',
    cpu_util: 'Загрузка CPU',
    data_channel: 'Канал данных',
    source: 'Источник',
    wait_task_end: 'Ожидание завершения задачи',
    mtp_acceptance: 'Принятие MTP-черновика',
    remaining: 'Осталось',
    used: 'Использовано',
    no_data_ssh: 'Данные недоступны (SSH отключён)',
    mem_used: 'ОЗУ исп.',
    total: 'Всего'
  },
  llamaState: {
    log_live: 'Лог: онлайн',
    log_unavailable: 'Лог недоступен',
    running: 'Выполняется',
    task_id: 'ID задачи',
    child_task: 'дочерняя задача',
    slot: 'Слот',
    prompt_speed: 'Скорость запроса',
    prompt_processed: 'Запрос обработан',
    elapsed: 'Затрачено',
    prompt_total: 'Всего запросов',
    eta_remaining: 'Осталось времени',
    context_usage: 'Использование контекста',
    kv_cache_hit: 'Попадание KV-кэша',
    decoded: 'Декодировано',
    realtime_speed: 'Скорость в реальном времени',
    avg_speed: 'Средняя скорость',
    eta_completion: 'Время до завершения',
    mtp_acceptance_rate: 'Коэф. принятия MTP',
    remaining_tokens: 'Осталось токенов',
    cache_hit: 'Попадание кэша',
    graphs_reused: 'Графов повторно',
    idle: 'Ожидание',
    log_unavailable_api: '⌁ Логи недоступны (API не готов)',
    offline: 'Офлайн',
    unknown: 'Неизвестно',
    prompt_processing: 'Обработка запроса',
    generating: 'Генерация',
    unlimited: 'Без ограничений',
    spec_decoding: 'Spec decoding',
    kv_cache: 'KV-кэш',
    batch_size: 'Размер батча',
    gpu_layers: 'Слои GPU',
    last_task: 'Последняя задача'
  },
  hostDetail: {
    unreachable: 'Хост недоступен (SSH отключён)',
    overview: 'Обзор',
    gen_speed: 'Скорость генерации',
    prompt_speed: 'Скорость запроса',
    context_usage: 'Использование контекста',
    mtp_acceptance: 'Принятие MTP',
    gpu: 'GPU',
    live_tasks: 'Активные задачи',
    system_resources: 'Системные ресурсы',
    model_and_slots: 'Модель и слоты',
    processes: 'Процессы',
    trends: 'Тренды',
    paused: 'Пауза',
    used: 'Использовано',
    buff_cache: 'Буф/Кэш',
    downlink: 'Приём',
    uplink: 'Передача',
    gpu_util: 'Загрузка GPU',
    gpu_mem: 'Память GPU',
    gpu_temp: 'Темп. GPU',
    gpu_power: 'Мощность GPU',
    cpu: 'CPU',
    memory: 'ОЗУ',
    network: 'Сеть',
    load_avg: 'Нагрузка',
    no_data_ssh: 'Данные недоступны (SSH отключён)',
    wait_task_end: 'Ожидание завершения задачи',
    truncated: 'Усечён',
    source: 'Источник',
    task_avg_speed: 'Ср. скор. задачи',
    last: 'Последний',
    remaining_short: 'Осталось',
    progress: 'Прогресс',
    eta_remaining: 'Осталось времени',
    system_label: 'Система'
  },
  barCard: {
    '60s_stats': '60с',
    peak: 'пик',
    avg: 'ср'
  },
  brandBar: {
    hosts: 'Хосты',
    active: 'Активные',
    connected: 'Подключён',
    disconnected: 'Отключён',
    speed: 'Скорость'
  }
}
