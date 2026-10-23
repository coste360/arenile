# Translation glossary (it → en / de / fr)

Italian is the source language of every user-facing string (spec 20). This
glossary fixes the terms used in the English, German and French translations of
the panel, the public booking page and the emails, so that the same Italian word
is always rendered the same way. Add a row here before introducing a new domain
term in a translation.

## Rules

- **Register.** Public-facing text (booking page, manage page, emails) is
  formal: German *Sie*, French *vous*. The panel uses the same formal register,
  so a string can move between the two without a rewrite. `npm run lint`
  (`web/scripts/i18n-check.mjs`) rejects informal German (*du, dein, dich…*)
  and French (*tu, ton, ta…*) pronouns; imperatives are checked in review
  (German: *Wählen Sie*, not *Wähle*).
- **English** is British English (*colour, catalogue, anonymise, organisation,
  cancelled*).
- **German** capitalises a full sentence after a colon (*… gesperrt: Der Platz
  ist buchbar.*) and uses „…“ quotes. **French** puts a narrow space before
  `: ; ? !` (written as a normal space in the JSON) and uses « … » quotes.
- **Numbers, dates and money** are never written into a translation: the code
  formats them with `Intl` (web) or `notify/format.go` (emails) and passes them
  as variables (`{{amount}}`, `{{date}}`). A percentage written in the text
  follows the language: `{{percent}}%` in English and Italian, `{{percent}} %`
  in German and French.
- **Plurals** use i18next suffixes (`_one`, `_other`; CLDR rules via
  `Intl.PluralRules`). The `_one` form may omit `{{count}}` (*Letzter Platz*);
  the `_other` form must show it. ICU message syntax (`{n, plural, …}`) is not
  used.
- **Placeholders** (`{{name}}`), literal tokens (`{row}{number}`) and Trans tags
  (`<0>…</0>`) are copied unchanged; the check fails if a translation drops or
  adds one.
- Product and brand names stay as they are: Arenile, Coste360, Stripe, Stripe
  Checkout, iPhone, Android, WhatsApp.

## Beach establishment

| Italian | English | German | French |
| --- | --- | --- | --- |
| stabilimento balneare (customer-facing) | beach club | Strandbad | établissement (balnéaire) |
| stabilimento (panel: the account/tenant) | establishment | Betrieb | établissement |
| lido | lido | Strandbad | plage |
| spiaggia | beach | Strand | plage |
| mappa (della spiaggia) | map | Plan | plan |
| postazione | spot | Platz | emplacement |
| ombrellone | umbrella | Sonnenschirm | parasol |
| lettino | sunbed | Liege | transat |
| sdraio | deckchair | Liegestuhl | chaise longue |
| gazebo | gazebo | Pavillon | gazebo |
| cabina | changing cabin | Umkleidekabine | cabine |
| fila | row | Reihe | rangée |
| zona | zone (panel), area (public) | Zone (panel), Bereich (public) | zone |
| pool (inventario a quantità) | pool | Bestand | stock |
| quota online | online quota | Online-Kontingent | quota en ligne |
| passerella | walkway | Steg | passerelle |
| bagnino | lifeguard | Rettungsschwimmer | maître-nageur |

## Booking and pricing

| Italian | English | German | French |
| --- | --- | --- | --- |
| prenotazione | booking | Buchung | réservation |
| fascia (mattina / pomeriggio / giornata intera) | slot (panel), time of day (public) | Tageszeit | créneau |
| formula (giornata, settimana, stagione…) | duration | Dauer, Buchungsdauer | formule |
| pack (pannello) / pacchetto (pubblico) | pack / package | Paket | forfait |
| extra | extra | Extra | supplément |
| listino | price list | Preisliste | grille tarifaire |
| periodo tariffario | rate period | Tarifperiode | période tarifaire |
| bassa / media / alta stagione, picco | low / mid / high season, peak | Neben- / Zwischen- / Haupt- / Spitzensaison | basse / moyenne / haute / très haute saison |
| prezzo a durata fissa | fixed-duration price | Festpreis | prix forfaitaire |
| convenzione | agreement | Vereinbarung | convention |
| codice promozionale | promo code | Aktionscode | code promo |
| policy (di prenotazione) | (booking) policy | (Buchungs-)Richtlinie | politique (de réservation) |
| anticipo (lead time) | lead time | Vorlauf | délai |
| blocco temporaneo (hold) | hold | Vormerkung, vorläufige Reservierung | blocage (temporaire) |
| acconto | deposit | Anzahlung | acompte |
| saldo (da incassare) | balance (due) | Restbetrag; saldi aperti = offene Beträge | solde (à encaisser) |
| rimborso | refund | Erstattung | remboursement |
| credito (del cliente) | credit | Guthaben (balance), Gutschrift (an amount credited) | avoir |
| penale di modifica | change fee | Änderungsgebühr | frais de modification |
| no-show / non presentato | no-show | nicht erschienen | non présenté |

## Season passes, renewals, waitlist

| Italian | English | German | French |
| --- | --- | --- | --- |
| abbonamento stagionale | season pass | Saisonabo | abonnement saisonnier |
| abbonato | pass holder | Abonnent | abonné |
| rilascio (di un giorno) | release | Freigabe | libération |
| rivenduto | resold | weiterverkauft | revendu |
| rinnovo | renewal | Verlängerung | renouvellement |
| campagna di rinnovo | renewal campaign | Verlängerungskampagne | campagne de renouvellement |
| proposta di rinnovo | renewal proposal | Verlängerungsangebot | proposition de renouvellement |
| prelazione | pre-emption right | Vorkaufsrecht | droit de préemption |
| scadenza (della proposta) | deadline | Frist | date limite |
| nuova stagione / passaggio di stagione | new season / season rollover | neue Saison / Saisonwechsel | nouvelle saison / passage de saison |
| lista d'attesa | waitlist | Warteliste | liste d'attente |
| offerta (dalla lista d'attesa) | offer | Angebot | offre |

## Cash desk and fiscal

| Italian | English | German | French |
| --- | --- | --- | --- |
| cassa | cash desk | Kasse | caisse |
| sessione di cassa | cash session | Kassensitzung | session de caisse |
| fondo cassa | opening float | Anfangsbestand | fond de caisse |
| incasso | payment taken, takings | Zahlung, Einnahmen | encaissement |
| versamento / prelievo | cash in / cash out | Einlage / Entnahme | apport / retrait |
| chiusura Z | Z report | Z-Bericht | ticket Z |
| corrispettivi | daily takings (Italian corrispettivi) | Tageseinnahmen (Corrispettivi) | recettes journalières (corrispettivi) |
| aliquota IVA / imponibile | VAT rate / net | MwSt.-Satz / Netto | taux de TVA / HT |
| ponte fiscale | fiscal bridge | Fiskalbrücke | pont fiscal |

## Panel

| Italian | English | German | French |
| --- | --- | --- | --- |
| pannello | panel | Verwaltungsbereich (Panel in desktop texts) | panneau |
| Configurazione (menu) | Settings | Einstellungen | Configuration |
| configurazione guidata | guided setup | geführte Einrichtung | configuration guidée |
| Operatività | Operations | Tagesgeschäft | Exploitation |
| proprietario / gestore / cassiere | owner / manager / cashier | Inhaber / Manager / Kassierer | propriétaire / gérant / caissier |
| pagina pubblica | public page | öffentliche Seite | page publique |
| link di gestione (magic link) | manage link | Verwaltungslink | lien de gestion |
| condizioni di prenotazione | booking conditions | Buchungsbedingungen | conditions de réservation |
| informativa sulla privacy | privacy notice | Datenschutzhinweis | politique de confidentialité |
| anonimizzazione (GDPR) | anonymisation (GDPR) | Anonymisierung (DSGVO) | anonymisation (RGPD) |
