## 9. Policy di prenotazione

Le policy sono regole con un ambito (`scope`) e un insieme di vincoli (`rules`). Per una prenotazione si applica la policy con priorità più alta tra quelle il cui ambito corrisponde; i campi non specificati ereditano dalla policy di default dello stabilimento.

```json
{
  "min_lead_time":   {"days": 0, "cutoff_time": "20:00"},
  "max_lead_time_days": 120,
  "same_day_booking": {"enabled": true, "until": "13:00", "only_slot": "PM_after"},
  "duration": {
    "allowed_formulas": ["day", "multi_day", "week"],
    "min_days": 1,
    "max_days": 30,
    "fixed_start_weekday": null
  },
  "payment": {
    "mode": "full | deposit | none",
    "deposit": {"type": "percent", "value": 30},
    "balance_due": "on_arrival | days_before:7"
  },
  "cancellation": [
    {"until_days_before": 14, "refund_percent": 100, "as": "refund"},
    {"until_days_before": 3,  "refund_percent": 50,  "as": "credit"},
    {"until_days_before": 0,  "refund_percent": 0}
  ],
  "modification": {"allowed_until_days_before": 2, "fee": 0},
  "weather_policy": {"enabled": true, "as": "credit"},
  "approval_required": false,
  "max_spots_per_booking": 4,
  "allow_spot_change_during_stay": false,
  "customer_can_release_season_days": true,
  "season_release_credit": {"enabled": false, "percent": 50}
}
```

Esempi tipici configurabili senza codice:
- ad agosto online solo settimane da sabato a sabato: policy con `scope.rate_periods = ["peak"]`, `fixed_start_weekday = 6`, `allowed_formulas = ["week"]`;
- i gazebo richiedono approvazione: policy con `scope.packs = ["gazebo"]`, `approval_required = true`;
- online pagamento totale, alla cassa nessun vincolo: due policy distinte per canale.

Le policy applicate vengono salvate in `policy_snapshot` per rendere verificabili cancellazioni e rimborsi anche se il gestore cambia le regole dopo.

---

