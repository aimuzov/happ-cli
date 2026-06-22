<p align="center">
  <img src="assets/banner.png" alt="happ-cli" width="800" />
</p>

# happ-cli

[English](README.md) | **Русский**

Терминальный VPN-клиент, совместимый с профилями подписок [HAPP](https://happ.su).
Забирает подписку, парсит share-ссылки (VLESS / VMess / Trojan / Shadowsocks) и
поднимает соединение через встроенный
[xray-core](https://github.com/XTLS/Xray-core) — как локальный прокси, системный
прокси или полноценный системный VPN (TUN).

Единый самодостаточный бинарник: xray-core и tun2socks встроены, внешних бинарей
не требуется.

## Возможности

- **Совместимость с HAPP**: subscription-URL отдаёт base64-список share-ссылок;
  читаются метаданные из заголовков, которые понимает HAPP:
  `subscription-userinfo` (трафик/срок), `profile-title`,
  `profile-update-interval`, `support-url`. Запросы уходят с
  `User-Agent: Happ/1.0`, чтобы панель вернула формат, который ждёт HAPP
  (переопределяется флагом `--ua`).
- **Протоколы**: VLESS (включая Reality / XTLS Vision), VMess, Trojan,
  Shadowsocks. **Транспорты**: TCP, WebSocket, gRPC, HTTP/2.
  **Безопасность**: TLS, Reality.
- **Три способа завернуть трафик**:
  - `connect` — локальный SOCKS5 + HTTP прокси на `127.0.0.1` (без root);
  - `connect --system-proxy` — выставляет системный SOCKS + HTTP/HTTPS прокси
    macOS (нужен `sudo`), чтобы браузеры и большинство приложений шли через него
    без правки таблицы маршрутов — уживается с другим активным VPN;
  - `connect --mode tun` — полноценный системный VPN через utun (нужен `sudo`).

> **Примечание**: xray-core не умеет outbound для Hysteria2 / TUIC / WireGuard.
> Такие серверы всё равно парсятся и показываются (с пометкой `unsupported`), но
> подключиться к ним через xray нельзя (нужен движок на базе sing-box).

## Как это устроено

```
subscription URL
      │  profile.Fetch (User-Agent: Happ/1.0)
      ▼
base64-список ссылок ──► link.Parse ──► []link.Server
                                            │ xray.BuildConfig
                                            ▼
                                    конфиг xray-core (JSON)
                                            │ xray.Start (встроенное ядро)
              ┌─────────────────────────────┼─────────────────────────────┐
              ▼                             ▼                             ▼
      proxy: SOCKS5/HTTP          --system-proxy: networksetup     tun: tun2socks
      на 127.0.0.1                ставит системный SOCKS/HTTP       + таблица маршрутов
      (без root)                  (sudo)                           (sudo, utun)
```

## Установка

### mise (рекомендуется)

Готовые бинарники публикуются в GitHub Releases. Ставятся через
[mise](https://mise.jdx.dev) — без установки Go. Внутри архива бинарник называется
`happ` (не `happ-cli`), поэтому укажи `exe=happ`:

```sh
mise use -g "github:aimuzov/happ-cli[exe=happ]@latest"
```

или зафиксировать в `mise.toml`:

```toml
[tools]
"github:aimuzov/happ-cli" = { version = "latest", exe = "happ" }
```

Бэкенд `ubi` работает так же и с теми же релизами, если он тебе привычнее:
`ubi:aimuzov/happ-cli[exe=happ]`.

> При частых установках задай `MISE_GITHUB_TOKEN` (или `GITHUB_TOKEN`), чтобы не
> упереться в лимиты GitHub API.

### Ручная загрузка

Скачай архив под свою ОС/архитектуру со страницы
[Releases](https://github.com/aimuzov/happ-cli/releases), распакуй и положи
бинарник `happ` в `PATH`.

### Из исходников

```sh
git clone https://github.com/aimuzov/happ-cli
cd happ-cli
go build -o happ .   # нужен Go 1.26+
```

Полученный бинарник `happ` самодостаточен.

> **`go install github.com/aimuzov/happ-cli@latest` не работает.** Сборке нужна
> директива `replace` в `go.mod` (примиряет xray-core и tun2socks по gvisor), а
> `go install pkg@version` игнорирует `replace`. Используй готовый бинарник либо
> клонируй и собирай.

## Использование

### Подписки

```sh
happ sub add https://panel.example/sub/TOKEN --name myvpn   # добавить (станет активной)
happ sub list                                               # список подписок
happ sub update [name]                                      # обновить (по умолчанию активную)
happ sub use <name>                                         # сделать подписку активной
happ sub rm <name>                                          # удалить
```

`sub list` показывает трафик и срок из заголовков подписки:

```
ACTIVE  NAME    TITLE       SERVERS  TRAFFIC          EXPIRES
*       myvpn   My VPN      12       12.4 GB / 200 GB  2026-09-01
```

### Серверы

```sh
happ list           # серверы активной подписки
happ list --sub x   # серверы конкретной подписки
```

```
#  PROTOCOL                 ADDRESS              TAG
1  vless                    de.example:443       🇩🇪 Германия
2  trojan                   nl.example:443       🇳🇱 Нидерланды
3  hysteria2 (unsupported)  hy.example:443       Fast HY2
```

### Подключение

`connect` работает в foreground до прерывания `Ctrl+C`. Аргумент `selector`
выбирает сервер: пусто = первый, число = индекс (1-based) из `happ list`, либо
подстрока тега без учёта регистра.

```sh
happ connect                 # первый сервер, режим proxy
happ connect 2               # сервер №2
happ connect germany         # первый сервер с тегом, содержащим "germany"

sudo happ connect 1 --system-proxy   # браузеры/приложения через системный прокси (без правки маршрутов)
sudo happ connect 1 --mode tun       # полноценный системный VPN
```

В обычном proxy-режиме настрой приложения на `socks5://127.0.0.1:10808`
(в Firefox включи «Proxy DNS when using SOCKS v5»).

### Флаги `connect`

| Флаг             | По умолчанию | Назначение                                                 |
| ---------------- | ------------ | ---------------------------------------------------------- |
| `-m, --mode`     | `proxy`      | `proxy` или `tun`                                          |
| `--socks`        | `10808`      | порт локального SOCKS5                                     |
| `--http`         | `10809`      | порт локального HTTP (режим proxy)                         |
| `--system-proxy` | `false`      | выставить системный прокси macOS (режим proxy, нужен sudo) |
| `--sub`          | активная     | имя подписки                                               |

### Три способа завернуть трафик — сравнение

- **`connect` (proxy)** — только приложения, явно настроенные на
  `socks5://127.0.0.1:10808` (например, Firefox с remote DNS). Без root.
- **`connect --system-proxy`** — выставляет на всех включённых сетевых сервисах
  системный SOCKS (порт `--socks`) и HTTP/HTTPS (порт `--http`), поэтому
  Safari/Chrome и приложения, игнорирующие SOCKS, идут через прокси. **Не трогает**
  таблицу маршрутов, поэтому **уживается с другим активным VPN**. Нужен `sudo`;
  прежние настройки прокси восстанавливаются при выходе. Если сессию убили
  (`kill -9`) и прокси завис — сбросить командой `sudo happ system-proxy off`.
- **`connect --mode tun`** — полноценный системный VPN через utun, перехватывает
  весь трафик. Нужен `sudo`. Если параллельно активен другой VPN — сначала
  отключи его, чтобы туннели не дрались за маршруты/DNS.

### Прочие команды

```sh
happ config [selector]       # вывести сгенерированный конфиг xray-core (отладка)
happ system-proxy off        # аварийный сброс системного прокси (sudo)
```

## Конфигурация и хранение

Состояние (подписки и кэш ссылок) хранится в `state.json` в конфиг-каталоге
пользователя (`~/Library/Application Support/happ-cli` на macOS),
переопределяется глобальным флагом `--home`.

## Детали режима TUN (macOS)

1. адрес сервера резолвится в IP, и на каждый добавляется host-маршрут к текущему
   next-hop (физический шлюз либо интерфейс уже активного VPN) — чтобы соединение
   прокси с сервером не зациклилось обратно в туннель;
2. создаётся устройство `utun`, и tun2socks форвардит его трафик на локальный
   SOCKS, который отдаёт xray;
3. дефолтный маршрут перекрывается двумя `/1`-маршрутами на utun (реальный default
   остаётся нетронутым для корректного отката);
4. глобальный IPv6 заворачивается в `lo0` (блокируется), чтобы IPv6-сайты
   (Google, YouTube) не утекали мимо туннеля — приложения откатываются на IPv4
   через туннель; link-local IPv6 продолжает работать по более специфичному
   маршруту;
5. при `Ctrl+C` все маршруты снимаются в обратном порядке.

## Ограничения

- **Hysteria2 / TUIC / WireGuard** xray-core не умеет (парсятся и показываются как
  `unsupported`). Большинство HAPP-профилей — VLESS-Reality, они работают
  полностью.
- **TUN и `--system-proxy` пока только для macOS**.
- **В режиме TUN IPv6 заблокирован** (путь прокси — IPv4); IPv6-only ресурсы во
  время подключения недоступны.
- `connect` работает в **foreground**; фонового демона пока нет.
- `kill -9` пропускает очистку: системный прокси останется включённым
  (`sudo happ system-proxy off`), а IPv6-блок-маршруты TUN — в таблице
  (`sudo route -n delete -inet6 -net ::/1; sudo route -n delete -inet6 -net
8000::/1`). Обычный `Ctrl+C` всё убирает сам.

## Структура проекта

| Пакет               | Назначение                                                   |
| ------------------- | ------------------------------------------------------------ |
| `internal/link`     | парсинг share-ссылок (vless/vmess/trojan/ss/hysteria2)       |
| `internal/profile`  | загрузка подписки, декод base64-списка + заголовков          |
| `internal/xray`     | сборка конфига xray-core из сервера, запуск встроенного ядра |
| `internal/tunnel`   | режим TUN: tun2socks + управление маршрутами macOS           |
| `internal/sysproxy` | системный прокси macOS через networksetup                    |
| `internal/store`    | хранение подписок и кэша ссылок                              |
| `internal/cli`      | команды cobra                                                |

## Разработка

```sh
go test ./...        # юнит-тесты + реальный end-to-end тест прокси
go vet ./...
```

Интеграционный тест xray поднимает реальный Shadowsocks-сервер и клиента,
собранного из `link.Server`, и проверяет, что HTTP-запрос через SOCKS-inbound
клиента доходит до цели сквозь прокси.

> xray-core и tun2socks требуют разные версии `gvisor.dev/gvisor`; директива
> `replace` в `go.mod` фиксирует gvisor на версии, с которой собираются оба. Не
> удаляй её — см. комментарий рядом.

### Релизы

Релизы собирает [GoReleaser](https://goreleaser.com) в CI по пушу тега:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Workflow `release` (`.github/workflows/release.yml`) собирает бинарники
darwin/linux под amd64/arm64 и публикует их в GitHub Releases. Там сборка
учитывает `replace` из `go.mod` (happ-cli — главный модуль). Локальный прогон:
`goreleaser release --clean --snapshot`.
