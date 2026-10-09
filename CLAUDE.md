# CLAUDE.md

Pamäť projektu pre Claude Code. Načíta sa na začiatku každej relácie.
Na konci každého kroku aktualizuj sekciu „Stav“ v tom istom pull requeste.

## Projekt

Fork knižnice walk (Windows Application Library Kit pre Go), pôvodne
`github.com/lxn/walk`, ktorá už nie je udržiavaná. Licencia BSD 3-clause:
súbory `LICENSE` a `AUTHORS` musia zostať zachované.

- Modul: `github.com/lpintes/walk`, minimálne Go 1.23.
- Hlavný balík v koreni repa, deklaratívne API v `declarative/`,
  príklady v `examples/`, nástroj `tools/ui2walk`.

## Používateľ a komunikácia

- Komunikuj po slovensky.
- Používateľ používa čítačku obrazovky: žiadne vizualizácie, diagramy ani
  tabuľky, všetko popisuj textom a zoznamami.
- Prístupnosť (accessibility, UI Automation, MSAA) je pre projekt dôležitá,
  pri zmenách na ňu dávaj zvláštny pozor.

## Vývoj v cloude

- Kontajner beží na Linuxe. Kód sa dá iba skompilovať pre Windows, nie
  spustiť. Testovanie GUI robí používateľ na Windows.
- Kontrola pred commitom:
  - `GOOS=windows GOARCH=amd64 go build ./...`
  - `GOOS=windows GOARCH=386 go build ./...`
  - `GOOS=windows GOARCH=arm64 go build . ./declarative ./tools/...`
    (príklady na arm64 nejdú, ich `rsrc.syso` sú 386 COFF objekty)
  - `gofmt -l .` musí byť prázdny
  - `GOOS=windows go vet ./...` (zatiaľ má známe upozornenia, nepridávaj nové)
- Ak novšia závislosť vyžaduje vyššie Go než 1.23, zvoľ staršiu kompatibilnú
  verziu (napr. `golang.org/x/sys` v0.35.0).
- Postup: jeden logický krok = jedna relácia = jeden pull request do `master`.

## Plán modernizácie

1. Go modul a modernizácia syntaxe (`go.mod`, importy, `//go:build`, `any`).
2. Prevziať `github.com/lxn/win` do repa ako interný balík `internal/win`
   (licencia BSD) a odstrániť externú závislosť. Správanie sa nemá meniť.
3. Generátor Win32 deklarácií z metadát `Windows.Win32.winmd` (NuGet balík
   `Microsoft.Windows.SDK.Win32Metadata`) pomocou parsera
   `github.com/microsoft/go-winmd`. Generovať iba symboly, ktoré walk používa
   (približne 1435), a postupne nimi nahrádzať ručný kód v `internal/win`.
   Makrá ako `LOWORD`, `MAKEINTRESOURCE` alebo `FAILED` v metadátach nie sú,
   tie ostanú ručne písané. Pozor na štruktúry závislé od architektúry
   a na COM rozhrania, ktoré walk sám implementuje (WebView, OLE hosting).

## Stav

- Krok 1: hotový, pull request lpintes/walk#1.
- Krok 2: nezačatý.
- Krok 3: nezačatý.

## Známe problémy

- `go vet` hlási 69 upozornení „possible misuse of unsafe.Pointer“, väčšinou
  prevod `lParam` na ukazovateľ na štruktúru vo window procedúrach. Preveriť
  pri kroku 2 alebo 3.
- Príklady nemajú `rsrc.syso` pre arm64.
