import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import './App.css'
import { Card, KpiCard, StatusBadge } from './components/ui'

type JsonObject = Record<string, unknown>
type JsonValue = JsonObject | unknown[] | string | number | boolean | null
type TabId = 'operacion' | 'productos' | 'primas' | 'archivos' | 'polizas'

const defaultProductPayload = `{
  "id": "mapfre_vida_custom",
  "code": "MAPFRE_VIDA",
  "insurer": "MAPFRE",
  "sheet_name": "Hoja1",
  "header_row": 1,
  "mappings": [
    { "canonical_field": "document_number", "source_header": "IDENTIFICACIONAFILIADO", "required": true },
    { "canonical_field": "birth_date", "source_header": "FECHANACIMIENTO", "required": true },
    { "canonical_field": "monthly_premium", "source_header": "PRIMAMENSUALPERIODO", "required": true },
    { "canonical_field": "credit_number", "source_header": "NUMEROPRESTAMO", "required": true }
  ],
  "rules": [
    { "type": "required_not_empty", "field": "document_number", "params": {} },
    { "type": "required_not_empty", "field": "birth_date", "params": {} },
    { "type": "number_gte", "field": "monthly_premium", "params": { "min": 0 } }
  ]
}`

const defaultFormatPayload = `{
  "id": "mapfre_vida_fmt_alt_1",
  "product_id": "mapfre_vida",
  "name": "layout alterno 1",
  "file_prefix": "INCLUSION-VIDA-MAPFRE-ALT",
  "sheet_name": "Hoja1",
  "header_row": 1,
  "priority": 90,
  "active": true,
  "mappings": [
    { "canonical_field": "document_number", "source_header": "IDENTIFICACIONAFILIADO", "required": true },
    { "canonical_field": "birth_date", "source_header": "FECHANACIMIENTO", "required": true },
    { "canonical_field": "monthly_premium", "source_header": "PRIMAMENSUALPERIODO", "required": true },
    { "canonical_field": "credit_number", "source_header": "NUMEROPRESTAMO", "required": true }
  ],
  "rules": [
    { "type": "required_not_empty", "field": "document_number", "params": {} },
    { "type": "required_not_empty", "field": "birth_date", "params": {} },
    { "type": "number_gte", "field": "monthly_premium", "params": { "min": 0 } }
  ]
}`

const tabs: Array<{ id: TabId; label: string; desc: string }> = [
  { id: 'operacion', label: 'Operación', desc: 'Runbook y monitoreo de procesos' },
  { id: 'productos', label: 'Productos', desc: 'Gestión de productos y reglas' },
  { id: 'primas', label: 'Primas', desc: 'Catálogos permitidos por producto' },
  { id: 'archivos', label: 'Archivos', desc: 'Seguimiento y descarga de cargas' },
  { id: 'polizas', label: 'Pólizas', desc: 'Consulta y revisión de resultados' },
]

function App() {
  const [activeTab, setActiveTab] = useState<TabId>('operacion')
  const [apiBaseUrl, setApiBaseUrl] = useState('/api/v1')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [lastResponse, setLastResponse] = useState<JsonValue | null>(null)
  const [flash, setFlash] = useState('')

  const [health, setHealth] = useState<JsonObject | null>(null)
  const [progress, setProgress] = useState<JsonObject | null>(null)
  const [products, setProducts] = useState<JsonObject[]>([])
  const [files, setFiles] = useState<JsonObject[]>([])
  const [policies, setPolicies] = useState<JsonObject[]>([])

  const [premiumProductId, setPremiumProductId] = useState('mapfre_acc_men')
  const [premiumCsv, setPremiumCsv] = useState('7800,7410,10600,10070')
  const [singlePremium, setSinglePremium] = useState('12000')
  const [allowedPremiums, setAllowedPremiums] = useState<JsonObject | null>(null)

  const [summaryFileId, setSummaryFileId] = useState('')
  const [downloadFileId, setDownloadFileId] = useState('')
  const [fileSummary, setFileSummary] = useState<JsonObject | null>(null)
  const [fileValidationReport, setFileValidationReport] = useState<JsonObject | null>(null)
  const [showValidationDrawer, setShowValidationDrawer] = useState(false)

  const [productPayload, setProductPayload] = useState(defaultProductPayload)
  const [editingProductId, setEditingProductId] = useState('')
  const [formatPayload, setFormatPayload] = useState(defaultFormatPayload)
  const [formats, setFormats] = useState<JsonObject[]>([])
  const [formatFilterProductId, setFormatFilterProductId] = useState('')
  const [matchFileName, setMatchFileName] = useState('')
  const [matchProductId, setMatchProductId] = useState('')
  const [matchHeadersCsv, setMatchHeadersCsv] = useState('')
  const [formatMatchResult, setFormatMatchResult] = useState<JsonObject | null>(null)
  const [quickProductId, setQuickProductId] = useState('')
  const [quickProductCode, setQuickProductCode] = useState('')
  const [quickProductInsurer, setQuickProductInsurer] = useState('')
  const [wizardProductId, setWizardProductId] = useState('')

  const [searchProductId, setSearchProductId] = useState('')
  const [searchDocument, setSearchDocument] = useState('')
  const [searchCredit, setSearchCredit] = useState('')
  const [searchPage, setSearchPage] = useState(1)
  const [searchPageSize, setSearchPageSize] = useState(50)
  const [includeRawPolicies, setIncludeRawPolicies] = useState(false)
  const [fileFilterStatus, setFileFilterStatus] = useState('')
  const [fileFilterProduct, setFileFilterProduct] = useState('')
  const [autoRefreshProgress, setAutoRefreshProgress] = useState(false)
  const [selectedErrorDetail, setSelectedErrorDetail] = useState('')
  const [showErrorDrawer, setShowErrorDrawer] = useState(false)

  const downloadUrl = useMemo(() => {
    if (!downloadFileId.trim()) return ''
    return `${apiBaseUrl}/files/download?file_id=${encodeURIComponent(downloadFileId.trim())}`
  }, [apiBaseUrl, downloadFileId])

  const progressItems = useMemo(() => {
    if (!progress || !Array.isArray(progress.items)) return []
    return progress.items.filter((item): item is JsonObject => !!item && typeof item === 'object')
  }, [progress])

  const visibleFiles = useMemo(() => {
    return files.filter((file) => {
      const status = String(file.status ?? '')
      const product = String(file.product_id ?? '')
      const matchesStatus = !fileFilterStatus || status.toUpperCase() === fileFilterStatus.toUpperCase()
      const matchesProduct = !fileFilterProduct || product.toLowerCase().includes(fileFilterProduct.toLowerCase())
      return matchesStatus && matchesProduct
    })
  }, [files, fileFilterProduct, fileFilterStatus])

  const fileStatusSummary = useMemo(() => {
    const summary: Record<string, number> = {}
    for (const file of files) {
      const status = String(file.status ?? 'UNKNOWN').toUpperCase()
      summary[status] = (summary[status] ?? 0) + 1
    }
    return summary
  }, [files])
  const recentIncidents = useMemo(() => {
    const fromProgress = progressItems
      .filter((item) => String(item.last_error ?? '').trim())
      .map((item) => ({
        source: 'progress',
        fileName: String(item.file_name ?? '-'),
        status: String(item.status ?? '-'),
        detail: String(item.last_error ?? '-'),
        when: String(item.updated_at ?? ''),
      }))

    const fromFiles = files
      .filter((item) => String(item.error_reason ?? '').trim())
      .map((item) => ({
        source: 'file',
        fileName: String(item.file_name ?? '-'),
        status: String(item.status ?? '-'),
        detail: String(item.error_reason ?? '-'),
        when: String(item.processed_at ?? ''),
      }))

    return [...fromProgress, ...fromFiles]
      .sort((a, b) => (a.when < b.when ? 1 : -1))
      .slice(0, 6)
  }, [files, progressItems])

  const processChecklist = useMemo(
    () => [
      { label: 'API saludable', done: Boolean(health) },
      { label: 'Productos cargados', done: products.length > 0 },
      { label: 'Scan ejecutado', done: progressItems.length > 0 || files.length > 0 },
      { label: 'Archivos procesados', done: files.some((f) => String(f.status) === 'PROCESSED') },
      { label: 'Pólizas consultadas', done: policies.length > 0 },
    ],
    [health, products.length, progressItems.length, files, policies.length],
  )
  const completedSteps = useMemo(() => processChecklist.filter((x) => x.done).length, [processChecklist])
  const wizardSteps = useMemo(
    () => [
      { id: 1, label: 'Crear producto', done: products.some((p) => String(p.id ?? '') === wizardProductId && wizardProductId !== '') },
      {
        id: 2,
        label: 'Crear formato',
        done: formats.some((f) => String(f.product_id ?? '') === wizardProductId && wizardProductId !== ''),
      },
      { id: 3, label: 'Definir reglas formato', done: Boolean(formatPayload.includes('"rules"')) },
      {
        id: 4,
        label: 'Configurar primas',
        done: premiumProductId.trim() === wizardProductId.trim() && Boolean(allowedPremiums),
      },
      { id: 5, label: 'Probar match', done: Boolean(formatMatchResult) },
    ],
    [allowedPremiums, formatMatchResult, formatPayload, formats, premiumProductId, products, wizardProductId],
  )
  const wizardCurrentStep = useMemo(() => wizardSteps.find((s) => !s.done)?.id ?? 5, [wizardSteps])

  useEffect(() => {
    if (!autoRefreshProgress || activeTab !== 'operacion') return
    const timer = setInterval(() => {
      void loadProgress()
      void loadFiles()
    }, 5000)
    return () => clearInterval(timer)
  }, [autoRefreshProgress, activeTab])

  async function callApi(path: string, init?: RequestInit): Promise<JsonValue | null> {
    setLoading(true)
    setError('')
    setFlash('')
    try {
      const response = await fetch(`${apiBaseUrl}${path}`, {
        ...init,
        headers: {
          'Content-Type': 'application/json',
          ...(init?.headers ?? {}),
        },
      })

      const text = await response.text()
      const maybeJson = text ? (JSON.parse(text) as JsonValue) : { ok: true }
      if (!response.ok) {
        throw new Error(
          typeof maybeJson === 'object' && maybeJson && 'error' in maybeJson
            ? String(maybeJson.error)
            : `HTTP ${response.status}`,
        )
      }
      setLastResponse(maybeJson)
      return maybeJson
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error inesperado')
      return null
    } finally {
      setLoading(false)
    }
  }

  function parsePremiums(csv: string): number[] {
    return csv
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
      .map((item) => Number(item))
      .filter((item) => Number.isFinite(item))
  }

  function patchFormatPayload(mutator: (obj: Record<string, unknown>) => void) {
    try {
      const parsed = JSON.parse(formatPayload) as Record<string, unknown>
      mutator(parsed)
      setFormatPayload(JSON.stringify(parsed, null, 2))
    } catch {
      setError('El JSON de formato no es válido para aplicar acción rápida')
    }
  }

  function addRequiredRuleToFormat(field: string) {
    patchFormatPayload((obj) => {
      const rules = Array.isArray(obj.rules) ? (obj.rules as Array<Record<string, unknown>>) : []
      const exists = rules.some((r) => String(r.type ?? '').toLowerCase() === 'required_not_empty' && String(r.field ?? '') === field)
      if (!exists) rules.push({ type: 'required_not_empty', field, params: {} })
      obj.rules = rules
    })
  }

  function addPersonMappingsToFormat() {
    const personMappings = [
      { canonical_field: 'full_name', source_header: 'NOMBRE AP', required: false },
      { canonical_field: 'gender', source_header: 'GENERO', required: false },
      { canonical_field: 'email', source_header: 'CORREO', required: false },
      { canonical_field: 'phone', source_header: 'TELEFONO', required: false },
      { canonical_field: 'address', source_header: 'DIRECCION', required: false },
      { canonical_field: 'city', source_header: 'OFICINA', required: false },
    ]
    patchFormatPayload((obj) => {
      const mappings = Array.isArray(obj.mappings) ? (obj.mappings as Array<Record<string, unknown>>) : []
      for (const pm of personMappings) {
        const exists = mappings.some((m) => String(m.canonical_field ?? '') === pm.canonical_field)
        if (!exists) mappings.push(pm)
      }
      obj.mappings = mappings
    })
    setFlash('Se agregaron mappings opcionales de persona al formato')
  }

  async function loadHealth() {
    const response = await callApi('/health')
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setHealth(response)
      setFlash('Salud de API consultada')
    }
  }

  async function seedProducts() {
    const response = await callApi('/bootstrap/sample-products', { method: 'POST' })
    if (response) setFlash('Seed ejecutada')
  }

  async function loadProducts() {
    const response = await callApi('/products')
    if (Array.isArray(response)) {
      setProducts(response.filter((item): item is JsonObject => !!item && typeof item === 'object'))
      setFlash('Productos actualizados')
    }
  }

  async function upsertProduct() {
    try {
      const payload = JSON.parse(productPayload) as JsonObject
      if (editingProductId && String(payload.id ?? '') !== editingProductId) {
        setError(`En modo edición no puedes cambiar el id. Debe permanecer como: ${editingProductId}`)
        return
      }
      const response = await callApi('/products', { method: 'POST', body: JSON.stringify(payload) })
      if (response) {
        setFlash('Producto creado/actualizado')
        setEditingProductId('')
        await loadProducts()
      }
    } catch {
      setError('El JSON del producto no es válido')
    }
  }

  async function createQuickProduct() {
    const id = quickProductId.trim()
    const code = quickProductCode.trim()
    if (!id || !code) {
      setError('Para crear producto rápido, id y code son obligatorios')
      return
    }
    const payload: JsonObject = {
      id,
      code,
      insurer: quickProductInsurer.trim() || 'N/A',
      mappings: [],
      rules: [],
    }
    const response = await callApi('/products', { method: 'POST', body: JSON.stringify(payload) })
    if (response) {
      setFlash(`Producto creado: ${id}`)
      await loadProducts()
      setWizardProductId(id)
      setQuickProductId('')
      setQuickProductCode('')
      setQuickProductInsurer('')
    }
  }

  async function loadFormats() {
    const qs = formatFilterProductId.trim()
      ? `?product_id=${encodeURIComponent(formatFilterProductId.trim())}`
      : ''
    const response = await callApi(`/product-formats${qs}`)
    if (Array.isArray(response)) {
      setFormats(response.filter((item): item is JsonObject => !!item && typeof item === 'object'))
      setFlash('Formatos actualizados')
    }
  }

  async function upsertFormat() {
    try {
      const payload = JSON.parse(formatPayload) as JsonObject
      const response = await callApi('/product-formats', { method: 'POST', body: JSON.stringify(payload) })
      if (response) {
        setFlash('Formato creado/actualizado')
        await loadFormats()
      }
    } catch {
      setError('El JSON del formato no es válido')
    }
  }

  async function toggleFormatActive(formatID: string, active: boolean) {
    const response = await callApi('/product-formats/active', {
      method: 'PATCH',
      body: JSON.stringify({ id: formatID, active }),
    })
    if (response) {
      setFlash(`Formato ${formatID} ${active ? 'activado' : 'desactivado'}`)
      await loadFormats()
    }
  }

  async function runFormatMatchTest() {
    const headersRaw = matchHeadersCsv
      .split(',')
      .map((x) => x.trim())
      .filter(Boolean)
    if (!matchFileName.trim()) {
      setError('file_name es obligatorio para test de match')
      return
    }
    if (headersRaw.length === 0) {
      setError('Debes ingresar headers CSV para test de match')
      return
    }
    const response = await callApi('/product-formats/match-test', {
      method: 'POST',
      body: JSON.stringify({
        file_name: matchFileName.trim(),
        product_id: matchProductId.trim(),
        headers: headersRaw,
      }),
    })
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setFormatMatchResult(response)
      setFlash('Test de match ejecutado')
    }
  }

  async function loadAllowedPremiums() {
    const response = await callApi(
      `/products/allowed-premiums?product_id=${encodeURIComponent(premiumProductId.trim())}`,
    )
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setAllowedPremiums(response)
      setFlash('Primas consultadas')
    }
  }

  async function replacePremiums() {
    const response = await callApi('/products/allowed-premiums', {
      method: 'PUT',
      body: JSON.stringify({
        product_id: premiumProductId.trim(),
        premiums: parsePremiums(premiumCsv),
      }),
    })
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setAllowedPremiums(response)
      setFlash('Catálogo de primas reemplazado')
    }
  }

  async function addPremium() {
    const premium = Number(singlePremium)
    if (!Number.isFinite(premium)) {
      setError('La prima debe ser numérica')
      return
    }
    const response = await callApi('/products/allowed-premiums', {
      method: 'POST',
      body: JSON.stringify({ product_id: premiumProductId.trim(), premium }),
    })
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setAllowedPremiums(response)
      setFlash('Prima agregada')
    }
  }

  async function deletePremium() {
    const premium = Number(singlePremium)
    if (!Number.isFinite(premium)) {
      setError('La prima a eliminar debe ser numérica')
      return
    }
    const response = await callApi(
      `/products/allowed-premiums?product_id=${encodeURIComponent(
        premiumProductId.trim(),
      )}&premium=${encodeURIComponent(String(premium))}`,
      { method: 'DELETE' },
    )
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setAllowedPremiums(response)
      setFlash('Prima eliminada')
    }
  }

  async function loadProgress() {
    const response = await callApi('/process/progress')
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setProgress(response)
      setFlash('Progreso actualizado')
    }
  }

  async function triggerScan() {
    const response = await callApi('/process/scan', { method: 'POST' })
    if (response) setFlash('Scan encolado')
  }

  async function loadFiles() {
    const response = await callApi('/files')
    if (Array.isArray(response)) {
      setFiles(response.filter((item): item is JsonObject => !!item && typeof item === 'object'))
      setFlash('Listado de archivos actualizado')
    }
  }

  async function loadFileSummary() {
    const response = await callApi(`/files/summary?file_id=${encodeURIComponent(summaryFileId.trim())}`)
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setFileSummary(response)
      setFlash('Resumen de archivo consultado')
    }
  }

  async function loadFileValidationReport(fileId?: string) {
    const id = (fileId ?? summaryFileId).trim()
    if (!id) {
      setError('Indica file_id para el informe de validación')
      return
    }
    const response = await callApi(`/files/validation-report?file_id=${encodeURIComponent(id)}`)
    if (response && typeof response === 'object' && !Array.isArray(response)) {
      setFileValidationReport(response)
      setShowValidationDrawer(true)
      setFlash('Informe de validación cargado')
    }
  }

  async function retryFileById(fileID: string) {
    const trimmed = fileID.trim()
    if (!trimmed) return
    const response = await callApi(`/files/retry?file_id=${encodeURIComponent(trimmed)}`, { method: 'POST' })
    if (response) {
      setFlash(`Archivo reencolado: ${trimmed}`)
      await loadFiles()
      await loadProgress()
    }
  }

  function openErrorDetail(lines: string[]) {
    setSelectedErrorDetail(lines.join('\n'))
    setShowErrorDrawer(true)
  }

  async function onSearchPolicies(event: FormEvent) {
    event.preventDefault()
    const params = new URLSearchParams()
    if (searchProductId.trim()) params.set('product_id', searchProductId.trim())
    if (searchDocument.trim()) params.set('document_number', searchDocument.trim())
    if (searchCredit.trim()) params.set('credit_number', searchCredit.trim())
    params.set('page', String(searchPage))
    params.set('page_size', String(searchPageSize))
    if (includeRawPolicies) params.set('include_raw', '1')
    const response = await callApi(`/policies/search?${params.toString()}`)
    if (response && typeof response === 'object' && !Array.isArray(response) && Array.isArray(response.items)) {
      setPolicies(response.items.filter((item): item is JsonObject => !!item && typeof item === 'object'))
      setFlash('Búsqueda de pólizas completada')
    }
  }

  async function loadPoliciesByProduct() {
    if (!searchProductId.trim()) {
      setError('Para listar pólizas por producto, ingresa product_id')
      return
    }
    const params = new URLSearchParams()
    params.set('product_id', searchProductId.trim())
    params.set('limit', String(searchPageSize))
    if (includeRawPolicies) params.set('include_raw', '1')
    const response = await callApi(`/policies?${params.toString()}`)
    if (response && typeof response === 'object' && !Array.isArray(response) && Array.isArray(response.items)) {
      setPolicies(response.items.filter((item): item is JsonObject => !!item && typeof item === 'object'))
      setFlash('Listado de pólizas por producto cargado')
    }
  }

  function startEditProduct(product: JsonObject) {
    setEditingProductId(String(product.id ?? ''))
    setProductPayload(JSON.stringify(product, null, 2))
    setFlash(`Producto cargado para edición: ${String(product.id ?? '-')}`)
  }

  function prepareFormatTemplate(productId: string, productCode = '') {
    const pid = productId.trim()
    if (!pid) return
    const prefixBase = productCode.trim() || pid.toUpperCase()
    const template = {
      id: `${pid}_fmt_${Date.now()}`,
      product_id: pid,
      name: 'nuevo formato',
      file_prefix: `${prefixBase}_`,
      sheet_name: 'Hoja1',
      header_row: 1,
      priority: 100,
      active: true,
      mappings: [
        { canonical_field: 'document_number', source_header: 'DOCUMENTO', required: true },
        { canonical_field: 'credit_number', source_header: 'CREDITO', required: true },
        { canonical_field: 'monthly_premium', source_header: 'PRIMA', required: true },
      ],
      rules: [
        { type: 'required_not_empty', field: 'document_number', params: {} },
        { type: 'required_not_empty', field: 'credit_number', params: {} },
        { type: 'number_gte', field: 'monthly_premium', params: { min: 0 } },
      ],
    }
    setFormatPayload(JSON.stringify(template, null, 2))
    setFormatFilterProductId(pid)
    setFlash(`Plantilla de formato lista para producto: ${pid}`)
  }

  return (
    <main className="adminLayout">
      <aside className="sidebar">
        <div className="brand">
          <h1>Busk Admin</h1>
          <p>Panel de operación integral</p>
        </div>
        <label className="field">
          API Base
          <input value={apiBaseUrl} onChange={(e) => setApiBaseUrl(e.target.value)} />
        </label>
        <nav className="sideNav">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={activeTab === tab.id ? 'navItem active' : 'navItem'}
              onClick={() => setActiveTab(tab.id)}
            >
              <span>{tab.label}</span>
              <small>{tab.desc}</small>
            </button>
          ))}
        </nav>
      </aside>

      <section className="content">
        <Card className="topbar">
          <div>
            <h2>{tabs.find((t) => t.id === activeTab)?.label}</h2>
            <p>{tabs.find((t) => t.id === activeTab)?.desc}</p>
          </div>
          <div className="stats">
            <span className="chip">Productos: {products.length}</span>
            <span className="chip">Archivos: {files.length}</span>
            <span className="chip">Pólizas: {policies.length}</span>
            <span className="chip">API: {health ? 'OK' : 'Pendiente'}</span>
          </div>
        </Card>

        {recentIncidents.length > 0 ? (
          <Card className="incidentCard" title="Incidentes recientes" subtitle="Errores y alertas más recientes sin hacer scroll">
            <div className="incidentList">
              {recentIncidents.map((incident, idx) => (
                <button
                  key={`${incident.fileName}-${incident.when}-${idx}`}
                  type="button"
                  className="incidentItem"
                  onClick={() =>
                    openErrorDetail([
                      `Archivo: ${incident.fileName}`,
                      `Origen: ${incident.source}`,
                      `Estado: ${incident.status}`,
                      `Fecha: ${incident.when || '-'}`,
                      `Detalle: ${incident.detail}`,
                    ])
                  }
                >
                  <StatusBadge status={incident.status} />
                  <span className="incidentFile">{incident.fileName}</span>
                  <span className="incidentDetail">{incident.detail}</span>
                </button>
              ))}
            </div>
          </Card>
        ) : null}

        {activeTab === 'operacion' ? (
          <Card title="Operación E2E" subtitle="Ejecución del runbook y seguimiento en tiempo real">
          <div className="kpis">
            <KpiCard value={`${completedSteps}/5`} label="pasos completados" />
            <KpiCard value={progressItems.length} label="items en progreso" />
            <KpiCard value={visibleFiles.length} label="archivos visibles" />
          </div>
          <div className="checklist">
            {processChecklist.map((step) => (
              <div key={step.label} className={`step ${step.done ? 'done' : 'pending'}`}>
                <span>{step.done ? '●' : '○'}</span>
                <span>{step.label}</span>
              </div>
            ))}
          </div>
          <div className="row">
            <button onClick={() => void loadHealth()}>Health</button>
            <button onClick={() => void seedProducts()}>Seed</button>
            <button onClick={() => void triggerScan()}>Scan SFTP</button>
            <button onClick={() => void loadProgress()}>Progreso</button>
          </div>
          <div className="row">
            <button onClick={() => void loadProducts()}>Refrescar productos</button>
            <button onClick={() => void loadFiles()}>Refrescar archivos</button>
            <button
              type="button"
              className={autoRefreshProgress ? 'active' : ''}
              onClick={() => setAutoRefreshProgress((v) => !v)}
            >
              Auto refresh 5s {autoRefreshProgress ? 'ON' : 'OFF'}
            </button>
          </div>
          {progressItems.length > 0 ? (
            <div className="tableWrap">
              <table>
                <thead>
                  <tr>
                    <th>Archivo</th>
                    <th>Paso</th>
                    <th>%</th>
                    <th>Estado</th>
                    <th>Error</th>
                  </tr>
                </thead>
                <tbody>
                  {progressItems.map((item, idx) => (
                    <tr key={`${String(item.file_name ?? 'p')}-${idx}`}>
                      <td>
                        <span className="ellipsisCell" title={String(item.file_name ?? '-')}>
                          {String(item.file_name ?? '-')}
                        </span>
                      </td>
                      <td>{String(item.step ?? '-')}</td>
                      <td>{String(item.percent ?? '-')}</td>
                      <td>
                        <StatusBadge status={String(item.status ?? '-')} />
                      </td>
                      <td>
                        {String(item.last_error ?? '').trim() ? (
                          <button
                            type="button"
                            onClick={() =>
                              openErrorDetail([
                                `Archivo: ${String(item.file_name ?? '-')}`,
                                `Estado: ${String(item.status ?? '-')}`,
                                `Paso: ${String(item.step ?? '-')}`,
                                `Detalle: ${String(item.last_error ?? '-')}`,
                              ])
                            }
                          >
                            Ver error
                          </button>
                        ) : (
                          <span className="muted">-</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
          </Card>
        ) : null}

        {activeTab === 'productos' ? (
          <Card title="Gestión de Productos" subtitle="Alta, modificación y revisión del catálogo operativo">
          <Card title="Wizard de Configuración" subtitle="Flujo guiado para dejar un producto operable en minutos">
            <div className="row">
              <input
                placeholder="product_id del wizard"
                value={wizardProductId}
                onChange={(e) => setWizardProductId(e.target.value)}
              />
              <button
                type="button"
                onClick={() => {
                  if (!wizardProductId.trim()) {
                    setError('Define product_id en el wizard')
                    return
                  }
                  setQuickProductId(wizardProductId.trim())
                  setQuickProductCode(wizardProductId.trim().toUpperCase())
                  setQuickProductInsurer(wizardProductId.toUpperCase().includes('BOLIVAR') ? 'BOLIVAR' : 'MAPFRE')
                }}
              >
                Cargar datos rápidos
              </button>
            </div>
            <p className="muted">Paso actual sugerido: {wizardCurrentStep}</p>
            <div className="row">
              <button type="button" onClick={() => void createQuickProduct()}>
                Paso 1: Crear producto
              </button>
              <button
                type="button"
                onClick={() => {
                  prepareFormatTemplate(wizardProductId.trim(), wizardProductId.trim().toUpperCase())
                  setFormatFilterProductId(wizardProductId.trim())
                }}
              >
                Paso 2: Plantilla formato
              </button>
              <button
                type="button"
                onClick={() => {
                  addRequiredRuleToFormat('document_number')
                  addRequiredRuleToFormat('credit_number')
                  addRequiredRuleToFormat('birth_date')
                  addPersonMappingsToFormat()
                }}
              >
                Paso 3: Reglas base + persona
              </button>
            </div>
            <div className="row">
              <button
                type="button"
                onClick={() => {
                  setPremiumProductId(wizardProductId.trim())
                  void replacePremiums()
                }}
              >
                Paso 4: Cargar primas
              </button>
              <button
                type="button"
                onClick={() => {
                  setMatchProductId(wizardProductId.trim())
                  void runFormatMatchTest()
                }}
              >
                Paso 5: Probar match
              </button>
              <button
                type="button"
                onClick={() => {
                  void loadProducts()
                  void loadFormats()
                }}
              >
                Refrescar estado wizard
              </button>
            </div>
            <div className="checklist">
              {wizardSteps.map((step) => (
                <div key={step.id} className={`step ${step.done ? 'done' : 'pending'}`}>
                  <span>{step.done ? '●' : '○'}</span>
                  <span>{step.id}. {step.label}</span>
                </div>
              ))}
            </div>
          </Card>
          <div className="row">
            <button onClick={() => void loadProducts()}>Listar productos</button>
            <button onClick={() => void upsertProduct()}>Guardar producto (JSON)</button>
            <button onClick={() => void loadFormats()}>Listar formatos</button>
            <button onClick={() => void upsertFormat()}>Guardar formato (JSON)</button>
            <button
              type="button"
              onClick={() => {
                setEditingProductId('')
                setProductPayload(defaultProductPayload)
              }}
            >
              Nuevo (reset editor)
            </button>
          </div>
          <div className="row">
            <input
              placeholder="Nuevo producto: id"
              value={quickProductId}
              onChange={(e) => setQuickProductId(e.target.value)}
            />
            <input
              placeholder="Nuevo producto: code"
              value={quickProductCode}
              onChange={(e) => setQuickProductCode(e.target.value)}
            />
            <input
              placeholder="Nuevo producto: insurer"
              value={quickProductInsurer}
              onChange={(e) => setQuickProductInsurer(e.target.value)}
            />
            <button onClick={() => void createQuickProduct()}>Crear producto rápido</button>
          </div>
          <p className="muted">
            {editingProductId
              ? `Editando producto: ${editingProductId}`
              : 'Sin producto seleccionado para edición'}
          </p>
          {editingProductId ? (
            <p className="muted">Nota: en modo edición el campo `id` debe mantenerse igual para evitar cambiar la llave.</p>
          ) : null}
          <label className="field">
            JSON de producto (upsert)
            <textarea
              value={productPayload}
              onChange={(e) => setProductPayload(e.target.value)}
              rows={16}
            />
          </label>
          <div className="tableWrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Code</th>
                  <th>Aseguradora</th>
                  <th>Acción</th>
                </tr>
              </thead>
              <tbody>
                {products.map((product, idx) => (
                  <tr key={`${String(product.id ?? 'product')}-${idx}`}>
                    <td>{String(product.id ?? '-')}</td>
                    <td>{String(product.code ?? '-')}</td>
                    <td>{String(product.insurer ?? '-')}</td>
                    <td>
                      <button type="button" onClick={() => startEditProduct(product)}>
                        Editar JSON
                      </button>
                      <button
                        type="button"
                        onClick={() =>
                          prepareFormatTemplate(String(product.id ?? ''), String(product.code ?? ''))
                        }
                      >
                        Crear formato
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <label className="field">
            JSON de formato (upsert)
            <textarea value={formatPayload} onChange={(e) => setFormatPayload(e.target.value)} rows={16} />
          </label>
          <p className="muted">Tip: usa el wizard de arriba para cargar reglas base y datos de persona con un solo clic.</p>
          <div className="row">
            <input
              placeholder="Filtro formatos por product_id"
              value={formatFilterProductId}
              onChange={(e) => setFormatFilterProductId(e.target.value)}
            />
            <button onClick={() => void loadFormats()}>Aplicar filtro</button>
          </div>
          <div className="tableWrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Producto</th>
                  <th>Nombre</th>
                  <th>Prefijo</th>
                  <th>Priority</th>
                  <th>Activo</th>
                  <th>Acción</th>
                </tr>
              </thead>
              <tbody>
                {formats.map((format, idx) => {
                  const active = Boolean(format.active)
                  const formatID = String(format.id ?? '')
                  return (
                    <tr key={`${formatID}-${idx}`}>
                      <td>{formatID || '-'}</td>
                      <td>{String(format.product_id ?? '-')}</td>
                      <td>{String(format.name ?? '-')}</td>
                      <td>{String(format.file_prefix ?? '-')}</td>
                      <td>{String(format.priority ?? '-')}</td>
                      <td>{active ? 'Sí' : 'No'}</td>
                      <td>
                        <button type="button" onClick={() => setFormatPayload(JSON.stringify(format, null, 2))}>
                          Editar JSON
                        </button>
                        <button type="button" onClick={() => void toggleFormatActive(formatID, !active)}>
                          {active ? 'Desactivar' : 'Activar'}
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <div className="row">
            <input
              placeholder="file_name test"
              value={matchFileName}
              onChange={(e) => setMatchFileName(e.target.value)}
            />
            <input
              placeholder="product_id (opcional)"
              value={matchProductId}
              onChange={(e) => setMatchProductId(e.target.value)}
            />
            <input
              placeholder="headers CSV (A,B,C)"
              value={matchHeadersCsv}
              onChange={(e) => setMatchHeadersCsv(e.target.value)}
            />
            <button onClick={() => void runFormatMatchTest()}>Probar match formato</button>
          </div>
          {formatMatchResult ? <pre>{JSON.stringify(formatMatchResult, null, 2)}</pre> : null}
          </Card>
        ) : null}

        {activeTab === 'primas' ? (
          <Card title="Gestión de Primas Permitidas" subtitle="Control del catálogo de valores por producto">
          <div className="row">
            <input
              placeholder="product_id"
              value={premiumProductId}
              onChange={(e) => setPremiumProductId(e.target.value)}
            />
            <input
              placeholder="premiums CSV"
              value={premiumCsv}
              onChange={(e) => setPremiumCsv(e.target.value)}
            />
            <input value={singlePremium} onChange={(e) => setSinglePremium(e.target.value)} />
          </div>
          <div className="row">
            <button onClick={() => void loadAllowedPremiums()}>Consultar</button>
            <button onClick={() => void replacePremiums()}>Reemplazar catálogo</button>
            <button onClick={() => void addPremium()}>Agregar prima</button>
            <button onClick={() => void deletePremium()}>Eliminar prima</button>
          </div>
          {allowedPremiums ? <pre>{JSON.stringify(allowedPremiums, null, 2)}</pre> : null}
          </Card>
        ) : null}

        {activeTab === 'archivos' ? (
          <Card title="Gestión de Archivos" subtitle="Seguimiento, filtros y descarga de resultados procesados">
          <div className="kpis">
            <KpiCard value={files.length} label="Total archivos" />
            <KpiCard value={fileStatusSummary.PROCESSED ?? 0} label="PROCESSED" />
            <KpiCard value={fileStatusSummary.ERROR ?? 0} label="ERROR" />
            <KpiCard value={fileStatusSummary.SKIPPED ?? 0} label="SKIPPED" />
            <KpiCard value={fileStatusSummary.PROCESSING ?? 0} label="PROCESSING" />
            <KpiCard value={fileStatusSummary.QUEUED ?? 0} label="QUEUED" />
          </div>
          <div className="row">
            <span className="muted">Filtro rápido por estado:</span>
            <button type="button" onClick={() => setFileFilterStatus('')}>
              Todos
            </button>
            <button type="button" onClick={() => setFileFilterStatus('PROCESSED')}>
              PROCESSED
            </button>
            <button type="button" onClick={() => setFileFilterStatus('ERROR')}>
              ERROR
            </button>
            <button type="button" onClick={() => setFileFilterStatus('SKIPPED')}>
              SKIPPED
            </button>
            <button type="button" onClick={() => setFileFilterStatus('PROCESSING')}>
              PROCESSING
            </button>
            <button type="button" onClick={() => setFileFilterStatus('QUEUED')}>
              QUEUED
            </button>
          </div>
          <div className="row">
            <button onClick={() => void loadFiles()}>Listar archivos</button>
            <button onClick={() => void loadProgress()}>Ver progreso</button>
            <input
              placeholder="Filtro product_id"
              value={fileFilterProduct}
              onChange={(e) => setFileFilterProduct(e.target.value)}
            />
            <select value={fileFilterStatus} onChange={(e) => setFileFilterStatus(e.target.value)}>
              <option value="">Todos los estados</option>
              <option value="PROCESSED">PROCESSED</option>
              <option value="ERROR">ERROR</option>
              <option value="SKIPPED">SKIPPED</option>
              <option value="PENDING">PENDING</option>
            </select>
          </div>
          <div className="row">
            <input placeholder="file_id summary" value={summaryFileId} onChange={(e) => setSummaryFileId(e.target.value)} />
            <button onClick={() => void loadFileSummary()}>Consultar summary</button>
            <button type="button" onClick={() => void loadFileValidationReport()}>
              Informe validación
            </button>
          </div>
          <div className="row">
            <input placeholder="file_id descarga" value={downloadFileId} onChange={(e) => setDownloadFileId(e.target.value)} />
            {downloadUrl ? (
              <a href={downloadUrl} target="_blank" rel="noreferrer">
                Descargar archivo
              </a>
            ) : (
              <span className="muted">Ingresa file_id</span>
            )}
          </div>
          {fileSummary ? <pre>{JSON.stringify(fileSummary, null, 2)}</pre> : null}
          <div className="tableWrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Archivo</th>
                  <th>Producto</th>
                  <th>Estado</th>
                  <th>Error</th>
                  <th>Acciones</th>
                </tr>
              </thead>
              <tbody>
                {visibleFiles.map((file, idx) => {
                  const fileId = String(file.id ?? '')
                  return (
                    <tr key={fileId || `file-${idx}`}>
                      <td>{fileId || '-'}</td>
                      <td>
                        <span className="ellipsisCell" title={String(file.filename ?? '-')}>
                          {String(file.filename ?? '-')}
                        </span>
                      </td>
                      <td>{String(file.product_id ?? '-')}</td>
                      <td>
                        <StatusBadge status={String(file.status ?? '-')} />
                      </td>
                      <td>
                        {String(file.error_reason ?? '').trim() ? (
                          <button
                            type="button"
                            onClick={() =>
                              openErrorDetail([
                                `Archivo: ${String(file.filename ?? '-')}`,
                                `File ID: ${fileId || '-'}`,
                                `Estado: ${String(file.status ?? '-')}`,
                                `Detalle: ${String(file.error_reason ?? '-')}`,
                              ])
                            }
                          >
                            Ver error
                          </button>
                        ) : (
                          <span className="muted">-</span>
                        )}
                      </td>
                      <td>
                        <button
                          type="button"
                          onClick={() => {
                            setSummaryFileId(fileId)
                            setDownloadFileId(fileId)
                          }}
                        >
                          Usar ID
                        </button>
                        <button type="button" onClick={() => void loadFileValidationReport(fileId)}>
                          Informe
                        </button>
                        {String(file.status ?? '').toUpperCase() === 'ERROR' ? (
                          <button type="button" onClick={() => void retryFileById(fileId)}>
                            Reintentar
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          </Card>
        ) : null}

        {activeTab === 'polizas' ? (
          <Card title="Gestión de Pólizas" subtitle="Búsqueda avanzada y revisión de resultados de validación">
          <form onSubmit={(e) => void onSearchPolicies(e)}>
            <div className="row">
              <input
                placeholder="product_id (opcional)"
                value={searchProductId}
                onChange={(e) => setSearchProductId(e.target.value)}
              />
              <input
                placeholder="document_number"
                value={searchDocument}
                onChange={(e) => setSearchDocument(e.target.value)}
              />
              <input
                placeholder="credit_number"
                value={searchCredit}
                onChange={(e) => setSearchCredit(e.target.value)}
              />
            </div>
            <div className="row">
              <input
                type="number"
                min={1}
                value={searchPage}
                onChange={(e) => setSearchPage(Number(e.target.value))}
              />
              <input
                type="number"
                min={1}
                max={200}
                value={searchPageSize}
                onChange={(e) => setSearchPageSize(Number(e.target.value))}
              />
              <button type="submit">Buscar pólizas</button>
              <button type="button" onClick={() => void loadPoliciesByProduct()}>
                Listar por producto
              </button>
              <button type="button" onClick={() => setIncludeRawPolicies((v) => !v)}>
                include_raw {includeRawPolicies ? 'ON' : 'OFF'}
              </button>
            </div>
          </form>
          <div className="tableWrap">
            <table>
              <thead>
                <tr>
                  <th>Producto</th>
                  <th>Documento</th>
                  <th>Crédito</th>
                  <th>Estado</th>
                  <th>Fila</th>
                </tr>
              </thead>
              <tbody>
                {policies.map((policy, idx) => (
                  <tr key={`${policy.file_id ?? 'x'}-${idx}`}>
                    <td>{String(policy.product_id ?? '-')}</td>
                    <td>{String(policy.document_number ?? '-')}</td>
                    <td>{String(policy.credit_number ?? '-')}</td>
                    <td>{String(policy.status ?? policy.policy_status ?? '-')}</td>
                    <td>{String(policy.row_number ?? '-')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          </Card>
        ) : null}

        <Card title="Consola" subtitle="Respuesta cruda del endpoint ejecutado">
          {loading ? <p>Cargando...</p> : null}
          {flash ? <p className="ok">{flash}</p> : null}
          {error ? <p className="error">{error}</p> : null}
          {lastResponse ? (
            <pre>{JSON.stringify(lastResponse, null, 2)}</pre>
          ) : (
            <p className="muted">Sin respuesta aún</p>
          )}
        </Card>

      </section>

      {showErrorDrawer ? (
        <div className="errorDrawerBackdrop" onClick={() => setShowErrorDrawer(false)}>
          <aside className="errorDrawer" onClick={(e) => e.stopPropagation()}>
            <div className="row">
              <h3>Detalle de Error</h3>
              <button type="button" onClick={() => setShowErrorDrawer(false)}>
                Cerrar
              </button>
            </div>
            {selectedErrorDetail ? <pre>{selectedErrorDetail}</pre> : <p className="muted">Sin error seleccionado</p>}
          </aside>
        </div>
      ) : null}

      {showValidationDrawer ? (
        <div className="errorDrawerBackdrop" onClick={() => setShowValidationDrawer(false)}>
          <aside className="errorDrawer" style={{ maxWidth: 'min(920px, 96vw)' }} onClick={(e) => e.stopPropagation()}>
            <div className="row">
              <h3>Informe de validación</h3>
              <button type="button" onClick={() => setShowValidationDrawer(false)}>
                Cerrar
              </button>
            </div>
            {fileValidationReport ? (
              <>
                <p className="muted">
                  Archivo: {String(fileValidationReport.file_name ?? '-')} · Estado:{' '}
                  {String(fileValidationReport.file_status ?? '-')}
                  {String(fileValidationReport.product_id ?? '').trim()
                    ? ` · Producto: ${String(fileValidationReport.product_id)}`
                    : ''}
                  {typeof fileValidationReport.policy_row_count === 'number'
                    ? ` · Filas en policies: ${fileValidationReport.policy_row_count}`
                    : ''}
                  {String(fileValidationReport.processed_at ?? '').trim()
                    ? ` · Procesado: ${String(fileValidationReport.processed_at)}`
                    : ''}
                </p>
                {String(fileValidationReport.error_reason ?? '').trim() ? (
                  <div style={{ marginBottom: 12 }}>
                    <strong>Error del procesamiento</strong>
                    <pre className="error" style={{ whiteSpace: 'pre-wrap', marginTop: 8 }}>
                      {String(fileValidationReport.error_reason)}
                    </pre>
                  </div>
                ) : null}
                {Number(fileValidationReport.policy_row_count ?? 0) === 0 ? (
                  <p className="muted" style={{ marginBottom: 12 }}>
                    Duplicados y pendientes salen de <code>policies</code>. Con <strong>0</strong> filas persistidas las
                    tablas pueden ir vacías aunque el archivo esté en error.
                    {String(fileValidationReport.error_reason ?? '').trim()
                      ? ' Revisa el bloque «Error del procesamiento» arriba.'
                      : ' Si no aparece motivo aquí, usa «Ver error» en la fila del archivo (misma fuente en BD).'}
                  </p>
                ) : null}
                <h4>Créditos duplicados</h4>
                <div className="tableWrap" style={{ maxHeight: 240, overflow: 'auto' }}>
                  <table>
                    <thead>
                      <tr>
                        <th>Crédito</th>
                        <th>Veces</th>
                        <th>Filas (todas)</th>
                        <th>Filas duplicadas</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(Array.isArray(fileValidationReport.duplicate_credits)
                        ? fileValidationReport.duplicate_credits
                        : []
                      ).map((row: unknown, idx: number) => {
                        const r = row as JsonObject
                        return (
                          <tr key={`dup-${String(r.credit_number ?? idx)}`}>
                            <td>{String(r.credit_number ?? '-')}</td>
                            <td>{String(r.count ?? '-')}</td>
                            <td>{Array.isArray(r.row_numbers) ? (r.row_numbers as number[]).join(', ') : '-'}</td>
                            <td>
                              {Array.isArray(r.duplicate_row_numbers)
                                ? (r.duplicate_row_numbers as number[]).join(', ')
                                : '-'}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
                <h4>Validaciones pendientes por fila</h4>
                <div className="tableWrap" style={{ maxHeight: 320, overflow: 'auto' }}>
                  <table>
                    <thead>
                      <tr>
                        <th>Fila</th>
                        <th>Doc</th>
                        <th>Crédito</th>
                        <th>Estado</th>
                        <th>Notas</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(Array.isArray(fileValidationReport.pending_validations)
                        ? fileValidationReport.pending_validations
                        : []
                      ).map((row: unknown, idx: number) => {
                        const r = row as JsonObject
                        const notes = Array.isArray(r.notes) ? (r.notes as string[]).join(' | ') : '-'
                        return (
                          <tr key={`pend-${String(r.row_number ?? idx)}`}>
                            <td>{String(r.row_number ?? '-')}</td>
                            <td>{String(r.document_number ?? '-')}</td>
                            <td>{String(r.credit_number ?? '-')}</td>
                            <td>{String(r.policy_status ?? '-')}</td>
                            <td title={notes}>
                              <span className="ellipsisCell">{notes}</span>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
                <details>
                  <summary>JSON completo</summary>
                  <pre>{JSON.stringify(fileValidationReport, null, 2)}</pre>
                </details>
              </>
            ) : (
              <p className="muted">Sin informe cargado</p>
            )}
          </aside>
        </div>
      ) : null}
    </main>
  )
}

export default App
