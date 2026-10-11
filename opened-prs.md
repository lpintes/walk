# Otvorené pull requesty v lxn/walk: čo prevziať do forku

Stav k 11. 10. 2026. Pôvodný repozitár lxn/walk má 27 otvorených pull
requestov. Každý som prešiel (kód z vetvy `refs/pull/N/head`) a porovnal
s aktuálnym masterom forku lpintes/walk (commit 8fa8f08, po kroku 10).
Žiadny kód som zatiaľ neprebral, toto je iba odporúčanie.

Súhrn:

- Odporúčam prevziať: 5 (#838, #771, #552, #393, #810).
- Stojí za zváženie, ale treba prepracovať alebo rozhodnúť: 8 (#228,
  #657, #457, #826, #402, #456, #438, #450).
- Neprevziať: 14 (zastarané, už vo forku, nahradené niečím lepším alebo
  nekvalitné).

Prístupnosti sa priamo týkajú tri: #552 (ikona v oznamovacej oblasti,
kde chýba aktivácia klávesnicou), #393 (prepínanie zobrazenia hesla) a
#228 (globálne klávesové skratky).

PR sú v každej skupine zoradené od najdôležitejšieho.

## Odporúčam prevziať

### #838 commondialogs: match full width '|'

- Autor zx2c4 (WireGuard), 2026-04-20, jeden riadok v `commondialogs.go`.
- Chyba: `FileDialog` nahrádza znak `|` vo filtri súborov nulou, ale
  porovnáva iba dolný bajt UTF-16 znaku (`byte(c) == '|'`). Každý znak,
  ktorého kód končí bajtom 0x7C, sa preto tiež zmení na nulu a rozbije
  filter. Príklady: kórejské znaky, ale aj poľské „ż“ (U+017C). Lokalizovaný
  filter s takým znakom vedie k nezmyselnému zoznamu typov súborov.
- Vo forku chyba stále je (`commondialogs.go`, riadok 43).
- Oprava (`c == '|'`) nemení správanie pre žiadny iný vstup. Testovať na
  Windows stačí krátko, napríklad filtrom s textom „Súbory ż (*.txt)|*.txt“.
- Priorita: vysoká, malá a bezpečná oprava.

### #771 TreeView: vloženie položky bez rodiča

- Autor dedalqq, 2021-03-17, `treeview.go`.
- Chyba: obsluha udalosti `ItemInserted` v `TreeView.SetModel` volá
  `item.Parent().ChildCount()`. Pri položke na koreňovej úrovni je rodič
  `nil` a program spadne (panic). Vo forku chyba stále je
  (`treeview.go`, okolo riadku 176).
- PR pre koreňovú položku použije `TVI_LAST`. Lepšie bude nájsť jej
  poradie cez `model.RootCount` a `model.RootAt` rovnako, ako sa to robí
  s deťmi, aby sa nová koreňová položka zaradila na správne miesto.
- Priorita: vysoká, oprava pádu.

### #552 NotifyIcon: udalosť dvojkliku

- Autor jonny-shen, 2019-06-03, `notifyicon.go`.
- Pridáva udalosť `MouseDbClicked` pre `WM_LBUTTONDBLCLK` a
  `WM_RBUTTONDBLCLK`. Vo forku nič také nie je.
- Dôležitejšie z hľadiska prístupnosti: walk nastavuje
  `NOTIFYICON_VERSION` 3, takže keď používateľ klávesnice vyberie ikonu
  (Windows+B, šípky, Enter alebo medzerník), Explorer pošle
  `NIN_KEYSELECT`, pri kliknutí myšou aj `NIN_SELECT`. Walk ani jednu
  správu nespracúva, takže aplikácia nemá ako zareagovať na aktiváciu
  klávesnicou; funguje iba kontextová ponuka (Shift+F10 alebo kláves
  Aplikácie). Navyše kontextovú ponuku umiestňuje podľa polohy kurzora
  myši aj vtedy, keď ju vyvolala klávesnica.
- Odporúčanie: prevziať myšlienku, ale rozšíriť ju o novú udalosť
  „aktivovaná“ (`Activated`), ktorá príde pri `NIN_SELECT` aj
  `NIN_KEYSELECT`, a pri klávesnici umiestniť ponuku k ikone. Názov
  `MouseDbClicked` premenovať na `MouseDoubleClicked` (alebo podobne).
  Pridať aj do `declarative`.
- Treba otestovať na Windows s NVDA.
- Priorita: vysoká pre prístupnosť, stredne veľká zmena.

### #393 LineEdit: vlastnosť PasswordMode pre data binding

- Autor adongy, 2018-05-21, `lineedit.go`.
- Zaregistruje `PasswordMode` ako vlastnosť s udalosťou zmeny, takže v
  `declarative` sa dá napríklad naviazať na zaškrtávacie pole „Zobraziť
  heslo“. Vo forku je `PasswordMode` iba obyčajný `bool` bez väzby.
- Pre nevidiacich je prepínač „zobraziť heslo“ užitočný: čítač obrazovky
  v poli s heslom číta iba hviezdičky, takže skontrolovať zadané heslo sa
  inak nedá. `EM_SETPASSWORDCHAR` s nulou zároveň odstráni štýl
  `ES_PASSWORD`, takže MSAA stav „chránené“ sa zmení spolu so zobrazením.
- Pri prevzatí: publikovať udalosť iba pri skutočnej zmene, po zmene
  prekresliť pole (`InvalidateRect`) a v `declarative` zmeniť
  `PasswordMode` z `bool` na `Property`. Otestovať s NVDA, či po
  prepnutí oznámi zmenu.
- Priorita: stredná, malá zmena.

### #810 Gramatické opravy v README

- Autor AMetIR, 2022-07-23, štyri riadky v `README.mdown`.
- Všetky štyri vety sú vo forku stále rovnaké a opravy sú správne
  („Its“ na „It is“ a podobne).
- Odporúčam ich prevziať pri najbližšej úprave README pre fork.
- Priorita: nízka, triviálne.

## Stojí za zváženie (prepracovať alebo rozhodnúť)

### #228 Globálne klávesové skratky

- Autor bibainet, 2016, aktualizovaný 2021, 10 komentárov.
- Pridáva `RegisterGlobalHotKey` (`RegisterHotKey`), udalosť `Hotkey` na
  každom okne a príklad. Vo forku nič také nie je.
- Užitočné najmä pre nástroje, ktoré používajú nevidiaci (skratka
  funguje, aj keď má fokus iná aplikácia).
- Nedostatky: chýba `UnregisterHotKey` (pri zatvorení okna skratka
  zostane), chýba `MOD_WIN` a `MOD_NOREPEAT`, nevracia chybu, pridáva
  metódu do verejného rozhrania `Window` a súbor `.gitattributes`
  nesúvisí s témou.
- Odporúčanie: napísať nanovo podľa tohto vzoru, deklarácie cez
  generátor (`winmd.txt`), s odregistrovaním pri `Dispose`.
- Priorita: stredná.

### #657 Fallback pri zlyhaní LoadIconWithScaleDown

- Autor xlplbo, 2019. PR obsahuje tri commity: opravu príkladu
  `logview`, záložné volanie `LoadImage` v `Icon`, keď
  `LoadIconWithScaleDown` zlyhá, a premenovanie cesty balíka na vlastný
  fork (to je dôvod, prečo mení 164 súborov).
- Zmysel má iba fallback v `icon.go`: niektoré ikony (napríklad súbory
  .ico bez vhodnej veľkosti) `LoadIconWithScaleDown` nenačíta a walk
  vtedy vráti chybu. Vo forku fallback nie je.
- Premenovanie balíka neprevziať. Príklad `logview` vo forku prešiel
  smoke testom, jeho prepis netreba.
- Priorita: nízka až stredná, treba overiť na Windows s ikonou, pri
  ktorej to zlyhá.

### #457 Validátor NotEmpty

- Autor hippwn, 2019, `validators.go` a `declarative/validators.go`.
- Pridáva validátor, ktorý hlási prázdne textové pole. Vo forku sa to
  dá iba cez `Regexp` s výrazom `.+`, čo dá všeobecnú chybovú správu.
- PR robí `v.(string)` bez kontroly typu (panic pri inom type).
- Súvisí s krokom 10: validačné chyby sa zatiaľ neoznamujú čítaču
  obrazovky. Ak sa bude riešiť ohlasovanie validácie, je to vhodná
  chvíľa pridať aj tento validátor.
- Priorita: nízka.

### #826 RunWithContext

- Autor kukymbr, 2023, `form.go` a hlavné slučky.
- Pridáva `RunWithContext`, ktorý ukončí slučku správ, keď sa zruší
  `context`.
- Chyba návrhu: kontext sa kontroluje iba po prijatí ďalšej správy, takže
  ak nepríde žiadna správa, okno sa nezavrie. Vo verzii s cgo kontext
  ignoruje úplne.
- Ak by to bolo treba, správne riešenie je gorutina, ktorá pri
  `ctx.Done()` zavolá `Synchronize` a zatvorí formulár. Patrí skôr do
  kroku 9 (verejné API).
- Priorita: nízka.

### #402 NewTabWidgetWithStyle

- Autor dayvonjersen, 2018, päť riadkov v `tabwidget.go`.
- Umožní zadať štýly ovládacieho prvku záložiek (napríklad viac riadkov
  záložiek alebo záložky ako tlačidlá). Rovnaký vzor ako
  `NewCompositeWithStyle` a `NewListBoxWithStyle`, ktoré už existujú.
- Neškodné, ale nikto o to zatiaľ nežiadal. Ak áno, pridať aj do
  `declarative`.
- Priorita: nízka.

### #456 Oprava komentárov k funkciám

- Automatický bot CodeLingo, 2019.
- Väčšina zmien je štylisticky zlá („// RowCount: Called by...“), ale tri
  opravujú skutočne nesprávnu dokumentáciu, ktorá je vo forku stále:
  komentár `MaxValue` v `numberedit.go` hovorí „MinValue“, komentár
  `MouseUp` v `notifyicon.go` hovorí „MouseDown“ a komentár
  `SetTextColor` v `numberedit.go` hovorí „TextColor“.
- Odporúčanie: PR neprebrať, tieto tri komentáre opraviť samostatne
  (napríklad v ďalšej časti kroku 8).
- Priorita: nízka.

### #438 DisableMinimizeButton a SwitchToThisWindow

- Autor tinycedar, 2018, `window.go`.
- `DisableMinimizeButton` odstráni tlačidlo minimalizácie. Pridáva ho do
  rozhrania `Window` pre každé okno, aj pre ovládacie prvky, kde nedáva
  zmysel. Ak by to bolo treba, patrí to do `FormBase` (a do
  `declarative`).
- `SwitchToThisWindow` je funkcia, ktorú Microsoft neodporúča používať,
  a PR ju načítava ručne mimo `internal/win`. Neprevziať.
- Priorita: nízka.

### #450 Udalosť pripojenia a odpojenia zariadenia

- Autor xaxys, 2019.
- Pridáva udalosti pre `WM_DEVICECHANGE` (pripojenie a odpojenie
  zariadenia, napríklad USB kľúča) a vlastnosti `OnDeviceArrival` a
  `OnDeviceRemove` v `declarative`.
- Úzke použitie, vlastný typ udalosti mimo vzoru walk, konštanty ručne,
  bez údajov o zariadení. Ak by bolo treba, napísať nanovo, ideálne až
  s generickými udalosťami (krok 9).
- Priorita: nízka.

## Neprevziať

### Už vyriešené alebo zastarané

- #676 commondialogs: do not chdir after file dialog. Fork už
  `OFN_NOCHANGEDIR` používa (upstream to zrejme vyriešil iným commitom).
- #721 support go module. Fork má `go.mod` od kroku 1.
- #575 a #560 náhrada `strings.ReplaceAll` kvôli Go starším ako 1.11.
  Fork vyžaduje Go 1.23, nemá zmysel.
- #356 `CurrentIndex` v deklaratívnom `ListBox`. Fork ho už má ako
  `Property`.
- #98 `NewIconFromResource` s číselným ID. Fork má
  `NewIconFromResourceId`.
- #185 a #168 farby riadkov v `TableView` (2015, 2016). Nahradené
  rozhraním `CellStyler`, ktoré fork má a ktoré je všeobecnejšie.
- #107 „some features“ (2014). Zmes nesúvisiacich zmien (stavový riadok,
  `MaxSize`, bitmapa zo zdrojov, oprava ikony) na kóde starom 12 rokov.
  `MaxSize` aj `NewBitmapFromResource` už fork má.
- #683 oprava príkladu `imageviewer`. Obchádza problém zástupnou
  záložkou a odstraňuje panel nástrojov. Príklad vo forku prešiel smoke
  testom; ak sa pri otvorení obrázka ukáže chyba, opraviť ju priamo.

### Nevhodné

- #452 LineEdit: vlastný znak pre heslo. Mení podpis verejnej metódy
  `SetPasswordMode` z `bool` na `rune` (nekompatibilná zmena) a
  „magickú“ hodnotu -1. Ak by bol vlastný znak potrebný, pridať
  samostatnú metódu `SetPasswordChar`. Pre prístupnosť nemá význam.
- #784 OpenGL widget a časovače okien. Veľký (360 riadkov), úzko
  zameraný, vlastné načítanie `opengl32.dll` mimo `internal/win` a
  priložený binárny súbor iba pre amd64. Časovače cez `WM_TIMER` v
  `WindowBase` by sa mohli zraziť s časovačmi, ktoré `TableView` používa
  s pevnými ID. Na časovače stačí `time.AfterFunc` so `Synchronize`.
- #834 „This is fork from lxn/walk“ v README. Patrí do cudzieho forku.
- #727 rozšírenie README o príklad. Kód v príklade sa neskompiluje
  (chýbajúce zátvorky) a formátovanie je rozbité.

## Návrh ďalšieho postupu

1. Jeden menší pull request s opravami #838 a #771 (pády a chyby bez
   zmeny API), plus tri opravy komentárov z #456. Testovanie na Windows
   je krátke.
2. Samostatný krok pre prístupnosť `NotifyIcon`: aktivácia klávesnicou
   (`NIN_SELECT`, `NIN_KEYSELECT`), umiestnenie ponuky pri klávesnici a
   dvojklik z #552. Test s NVDA.
3. `PasswordMode` ako vlastnosť (#393), prípadne spolu s ohlasovaním
   validačných chýb a validátorom `NotEmpty` (#457).
4. Globálne klávesové skratky (#228), ak o ne bude záujem.
5. Gramatické opravy README (#810) pri najbližšej úprave README.
