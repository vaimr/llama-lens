export default {
  language: {
    label: 'Language',
    en: 'EN',
    ru: 'РУ'
  },
  brand: {
    name: 'LlamaLens',
    sub: 'llama-server real-time monitoring'
  },
  app: {
    title: 'LlamaLens · llama-server real-time monitoring'
  },
  portal: {
    noHosts: 'No hosts connected',
    waiting: 'Waiting for hosts...'
  },
  router: {
    portal: 'Portal',
    host: 'Host'
  },
  theme: {
    labels: {
      aurora: 'Aurora',
      terminal: 'Terminal',
      default: 'Default'
    }
  },
  hostStatus: {
    allHosts: 'All Hosts',
    nA: 'N/A',
    offline: 'Offline',
    connected: 'Connected',
    disconnected: 'Disconnected'
  },
  speed: {
    label: 'Speed',
    noActiveSlot: 'No active slot',
    noModelsLoaded: 'No models loaded'
  },
  hostCard: {
    model: 'Model',
    params: 'Params',
    speed: 'Speed',
    gpu: 'GPU',
    vram: 'VRAM',
    temp: 'Temp',
    power: 'Power',
    fan: 'Fan',
    cpu: 'CPU',
    mem: 'Mem',
    nA: 'N/A',
    noModelLoaded: 'No model loaded',
    noData: 'No data'
  },
  gpu: {
    utilization: 'GPU Utilization',
    memory: 'GPU Memory',
    temperature: 'Temperature',
    power: 'Power',
    fanSpeed: 'Fan Speed',
    gpu: 'GPU',
    percent: '%',
    celsius: '°C',
    watts: 'W'
  },
  cpu: {
    usage: 'CPU Usage',
    cores: 'Cores',
    loadAverage: 'Load Average',
    frequency: 'Frequency',
    cpu: 'CPU',
    percent: '%',
    cores: 'cores',
    load: 'load',
    mhz: 'MHz'
  },
  memory: {
    usage: 'Memory Usage',
    swap: 'Swap',
    cache: 'Cache',
    mem: 'Mem',
    percent: '%',
    gb: 'GB'
  },
  network: {
    label: 'Network',
    interface: 'Interface',
    rx: 'RX',
    tx: 'TX',
    mbps: 'Mbps'
  },
  disk: {
    label: 'Disk',
    mount: 'Mount',
    read: 'Read',
    write: 'Write',
    mbps: 'MB/s'
  },
  modelInfo: {
    path: 'Model Path',
    parameters: 'Parameters',
    embedding: 'Embedding',
    vocab: 'Vocab',
    nA: 'N/A'
  },
  llamaState: {
    phase: 'Phase',
    taskId: 'Task ID',
    promptProcessing: 'Prompt Processing',
    generation: 'Generation',
    idle: 'Idle',
    processing: 'Processing',
    generating: 'Generating',
    tokens: 'Tokens',
    prompt: 'Prompt',
    eval: 'Eval',
    total: 'Total',
    nA: 'N/A'
  },
  llamaProcess: {
    process: 'Process',
    pid: 'PID',
    cpu: 'CPU%',
    rss: 'RSS',
    vsz: 'VSZ',
    service: 'Service',
    nA: 'N/A'
  },
  slotTable: {
    slot: 'Slot',
    id: 'ID',
    context: 'Context',
    decoded: 'Decoded',
    speed: 'Speed',
    nA: 'N/A'
  },
  topProcessTable: {
    pid: 'PID',
    cpu: 'CPU%',
    mem: 'MEM%',
    rss: 'RSS',
    command: 'COMMAND'
  },
  eventFeed: {
    title: 'Event Stream',
    noEvents: 'No events yet',
    newEvent: 'New Event'
  },
  themeSwitcher: {
    label: 'Theme'
  },
  terminal: {
    prompt: '$ ',
    title: 'Terminal',
    connected: 'Connected',
    disconnected: 'Disconnected'
  },
  hostDetail: {
    title: 'Host Details',
    systemResources: 'System Resources',
    modelInfo: 'Model Info',
    llamaState: 'Llama State',
    processList: 'Process List',
    events: 'Events',
    gpu: 'GPU',
    cpu: 'CPU',
    memory: 'Memory',
    network: 'Network',
    disk: 'Disk'
  },
  topBar: {
    back: 'Back',
    speed: 'Speed',
    context: 'Context',
    memory: 'Memory',
    llama_offline: 'LLaMA offline',
    ssh_disconnected: 'SSH disconnected',
    online: 'Online',
    realtime: 'Real-time',
    paused: 'Paused',
    ws_disconnected: 'WS disconnected',
    util_temp_power_mem: 'Util / Temp / Power / Mem Used / Total',
    cpu_util: 'CPU Utilization',
    data_channel: 'Data channel',
    source: 'Source',
    wait_task_end: 'Waiting for task to end',
    mtp_acceptance: 'MTP draft acceptance',
    remaining: 'Remaining',
    used: 'Used',
    no_data_ssh: 'Data unavailable (SSH disconnected)',
    mem_used: 'Mem used',
    total: 'Total'
  },
  llamaState: {
    log_live: 'Log live',
    log_unavailable: 'Log unavailable',
    running: 'Running',
    task_id: 'Task ID',
    child_task: 'child task',
    slot: 'Slot',
    prompt_speed: 'Prompt speed',
    prompt_processed: 'Prompt processed',
    elapsed: 'Elapsed',
    prompt_total: 'Prompt total',
    eta_remaining: 'ETA remaining',
    context_usage: 'Context usage',
    kv_cache_hit: 'KV cache hit',
    decoded: 'Decoded',
    realtime_speed: 'Real-time speed',
    avg_speed: 'Avg speed',
    eta_completion: 'ETA completion',
    mtp_acceptance_rate: 'MTP acceptance rate',
    remaining_tokens: 'Remaining tokens',
    cache_hit: 'Cache hit',
    graphs_reused: 'Graphs reused',
    idle: 'Idle',
    log_unavailable_api: '⌁ Logs unavailable (API not ready)',
    offline: 'Offline',
    unknown: 'Unknown',
    prompt_processing: 'Prompt Processing',
    generating: 'Generating',
    unlimited: 'Unlimited',
    spec_decoding: 'Spec decoding',
    kv_cache: 'KV cache',
    batch_size: 'Batch size',
    gpu_layers: 'GPU layers',
    last_task: 'Last task'
  },
  hostDetail: {
    unreachable: 'Host unreachable (SSH down)',
    overview: 'Live Overview',
    gen_speed: 'Gen Speed',
    prompt_speed: 'Prompt Speed',
    context_usage: 'Context Usage',
    mtp_acceptance: 'MTP Acceptance',
    gpu: 'GPU',
    live_tasks: 'Live Tasks',
    system_resources: 'System Resources',
    model_and_slots: 'Model & Slots',
    processes: 'Processes',
    trends: 'Trends',
    paused: 'Paused',
    used: 'Used',
    buff_cache: 'Buff/Cache',
    downlink: 'Downlink',
    uplink: 'Uplink',
    gpu_util: 'GPU Util',
    gpu_mem: 'GPU Mem',
    gpu_temp: 'GPU Temp',
    gpu_power: 'GPU Power',
    cpu: 'CPU',
    memory: 'Memory',
    network: 'Network',
    load_avg: 'Load Avg',
    no_data_ssh: 'Data unavailable (SSH disconnected)',
    wait_task_end: 'Waiting for task to end',
    truncated: 'Truncated',
    source: 'Source',
    task_avg_speed: 'Task avg speed',
    last: 'Last',
    remaining_short: 'Remaining',
    progress: 'Progress',
    eta_remaining: 'ETA remaining',
    system_label: 'System'
  },
  barCard: {
    '60s_stats': '60s',
    peak: 'peak',
    avg: 'avg'
  },
  brandBar: {
    hosts: 'Hosts',
    active: 'Active',
    connected: 'Connected',
    disconnected: 'Disconnected',
    speed: 'Speed'
  }
}
