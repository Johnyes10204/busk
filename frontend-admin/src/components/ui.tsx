import type { ReactNode } from 'react'

type CardProps = {
  title?: string
  subtitle?: string
  children: ReactNode
  className?: string
}

type KpiCardProps = {
  value: string | number
  label: string
}

type StatusBadgeProps = {
  status: string
}

export function Card({ title, subtitle, children, className = '' }: CardProps) {
  return (
    <section className={`card ${className}`.trim()}>
      {title ? (
        <header className="sectionHeader">
          <h2>{title}</h2>
          {subtitle ? <p>{subtitle}</p> : null}
        </header>
      ) : null}
      {children}
    </section>
  )
}

export function KpiCard({ value, label }: KpiCardProps) {
  return (
    <div className="kpiCard">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  )
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const cssClass = `badge status-${status.toLowerCase()}`
  return <span className={cssClass}>{status}</span>
}
