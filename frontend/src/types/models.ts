export type AssetStatus = 'available' | 'in_use' | 'repair' | 'broken' | 'disposed'
export type TxType = 'out' | 'in' | 'transfer'
export type InspectionProfile = 'none' | 'general' | 'laptop'
export type SpecField = 'cpu' | 'ram' | 'storage' | 'os' | 'screen_size'

export interface Employee { id: number; name: string; nik: string | null; department: string | null; hp: string | null; email: string | null; address: string | null; created_at: string }
export interface Category { id: number; name: string; code: string; spec_fields: SpecField[]; inspection: InspectionProfile; created_at: string }
export interface AssetImage { id: number; asset_id: number; name: string }
export interface Asset {
  id: number; tag: string; category_id: number
  brand: string | null; model: string | null; serial_number: string | null
  cpu: string | null; ram: string | null; storage: string | null; os: string | null; screen_size: string | null
  manufacture_year: number | null; purchase_date: string | null; purchase_price: number | null
  warranty_until: string | null; vendor: string | null
  status: AssetStatus; location: string | null; holder_id: number | null; quantity: number; notes: string | null
  category?: Category; holder?: Employee | null; images: AssetImage[]
  created_at: string; updated_at: string
}
export interface User { id: number; name: string; email: string }
export interface TransactionDetail {
  id: number; transaction_id: number; asset_id: number
  prev_status: AssetStatus; new_status: AssetStatus; note: string | null
  asset?: Asset; transaction?: Transaction
}
export interface Transaction {
  id: number; order_number: string; type: TxType; employee_id: number; from_employee_id: number | null; note: string | null
  employee?: Employee; from_employee?: Employee | null; created_by?: User | null
  details?: TransactionDetail[]; inspections?: Inspection[]; created_at: string
}

// ---- physical check (inspection) ----
export type CheckResultValue = 'ok' | 'bad' | ''
export type Conclusion = 'layak' | 'perlu_perbaikan' | 'tidak_layak' | ''
export interface CheckResult { code: string; result: CheckResultValue; note: string }
export interface AccessoryResult { code: string; present: boolean | null; condition: string; note: string }
export interface InspectionData {
  profile: InspectionProfile
  items: CheckResult[]
  accessories: AccessoryResult[]
  battery_health: number | null
  conclusion: Conclusion
  notes: string
}
/** What the form sends for one asset. */
export interface InspectionInput extends Partial<InspectionData> {
  skipped?: boolean
  skip_reason?: string
}
export interface Inspection {
  id: number; transaction_id: number; asset_id: number; kind: TxType; employee_id: number
  from_employee_id: number | null; skipped: boolean; skip_reason: string | null; data: InspectionData
}
export interface CheckItem { code: string; label: string; battery?: boolean }
export interface InspectionTemplate {
  profile: InspectionProfile
  title: string
  sections: { key: string; title: string; items: CheckItem[] }[]
  accessories: { code: string; label: string }[]
}

// ---- condition report (who had the asset when a component went bad) ----
export interface ConditionFinding {
  type: 'new_damage' | 'fixed' | 'existing' | 'battery'
  code: string; label: string; note: string
  from_inspection_id: number; to_inspection_id: number
  responsible: { kind: 'employee' | 'storage' | 'unknown'; name?: string }
  explanation: string
}
export interface ConditionEvent {
  inspection_id: number; transaction_id: number; order_number: string; kind: TxType; at: string
  employee: { id: number; name: string } | null; from_employee: { id: number; name: string } | null
  skipped: boolean; skip_reason?: string; conclusion: Conclusion; battery_health: number | null
  bad_items: { code: string; label: string; note: string }[]; notes: string
}
export interface ConditionReport { events: ConditionEvent[]; findings: ConditionFinding[]; disclaimer: string }
export interface AssetLog {
  id: number; asset_id: number; action: string; from_status: AssetStatus | null; to_status: AssetStatus | null
  message: string | null; transaction_id: number | null; asset?: Asset; user?: User | null; created_at: string
}
export interface Option { id: number; text: string; isNew?: boolean }
