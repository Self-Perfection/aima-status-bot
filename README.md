# AIMA Renew Watch Bot

Telegram-бот, который мониторит страницу Validação на
`portal-renovacoes.aima.gov.pt` и присылает сообщение, когда меняется
числовой статус заявки на ВНЖ.

Дополняет [userscript](https://github.com/Self-Perfection/gov.pt_enhancement_userscripts):
тот показывает статус прямо на странице, бот — наблюдает фоном без
открытого браузера.

**Готовый бот:** [@aima_renew_watch_bot](https://t.me/aima_renew_watch_bot)

## ⚠️ Приватность

URL страницы Validação — это credential. По нему открывается ваша
персональная информация: имя, NIF, NISS, NIE, адрес, email, тип заявки.
Когда вы подписываете URL на мониторинг **в публичном инстансе бота**,
оператор бота и сервер технически имеют доступ к этим данным.

Митигации, реализованные в коде:

- URL хранится в БД зашифрованным AES-GCM (ключ — в `ENV`, не в БД).
- HTML-ответы AIMA не логируются и не сохраняются: из них
  вытаскивается только число статуса.
- При статусе 6 (Aprovado) подписка автоматически удаляется вместе со
  всем хранимым по ней.
- Команда `/forget_me` удаляет все данные пользователя мгновенно.

Если вы беспокоитесь о приватности — поднимите свой инстанс бота
(см. ниже).

## Команды

```
/start       — приветствие и дисклеймер
/agree       — подтвердить согласие на обработку данных
/add <url>   — подписаться на мониторинг (после /agree)
/list        — мои подписки
/remove <id> — отписаться
/forget_me   — удалить все мои данные
/help        — список команд
```

Лимит: 4 подписки на одного пользователя.

## Self-hosting

### Предварительно

- Go 1.24+
- Зарегистрированный бот в [@BotFather](https://t.me/BotFather), его токен
- 32 байта случайных данных в base64:

  ```sh
  openssl rand -base64 32
  ```

### Сборка

```sh
go build -o aima-renew-watch-bot ./cmd/bot
```

Бинарь самодостаточный: `modernc.org/sqlite` — pure-Go, CGO не нужен.

### Запуск

Переменные окружения:

| Имя                  | Обязат. | Описание                                              |
|----------------------|---------|-------------------------------------------------------|
| `BOT_TOKEN`          | да      | токен от @BotFather                                   |
| `ENC_KEY`            | да      | base64 32 байт для AES-256-GCM                        |
| `DB_PATH`            | нет     | путь к SQLite (по умолчанию `./bot.db`)               |
| `HEALTHCHECK_URL`    | нет     | URL для периодического GET (например, healthchecks.io) |
| `HEALTHCHECK_EVERY`  | нет     | интервал пинга healthcheck, Go duration (по умолч. `5m`) |

```sh
BOT_TOKEN=... ENC_KEY=... ./aima-renew-watch-bot
```

## Деплой (systemd, Linux)

Отдельный системный пользователь, бинарь в `/usr/local/bin`, БД в
`/var/lib`, секреты в env-файле в `/etc`. Все команды на сервере
выполняются под root.

### 1. Системный пользователь и директория для БД

```sh
useradd --system --home-dir /var/lib/aima-renew-watch-bot \
        --shell /usr/sbin/nologin aima-bot
install -d -o aima-bot -g aima-bot -m 700 /var/lib/aima-renew-watch-bot
```

### 2. Сборка и установка бинаря

На dev-машине (cross-compile под Linux/amd64 — CGO бот не использует):

```sh
GOOS=linux GOARCH=amd64 go build -o aima-renew-watch-bot ./cmd/bot
scp aima-renew-watch-bot user@server:/tmp/
```

На сервере:

```sh
install -m 755 /tmp/aima-renew-watch-bot /usr/local/bin/
```

### 3. Env-файл с секретами

```sh
install -d -m 750 -o root -g aima-bot /etc/aima-renew-watch-bot
cat > /etc/aima-renew-watch-bot/env <<EOF
BOT_TOKEN=<токен_от_botfather>
ENC_KEY=$(openssl rand -base64 32)
DB_PATH=/var/lib/aima-renew-watch-bot/bot.db
# опционально:
# HEALTHCHECK_URL=https://hc-ping.com/<uuid>
EOF
chown root:aima-bot /etc/aima-renew-watch-bot/env
chmod 640 /etc/aima-renew-watch-bot/env
```

⚠️ `ENC_KEY` после первого запуска **менять нельзя** — все сохранённые
URL станут нечитаемыми. Если вдруг сменили — `rm
/var/lib/aima-renew-watch-bot/bot.db` и попросите подписчиков
переподписаться.

### 4. systemd unit

`/etc/systemd/system/aima-renew-watch-bot.service`:

```ini
[Unit]
Description=AIMA Renew Watch Telegram bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=aima-bot
Group=aima-bot
EnvironmentFile=/etc/aima-renew-watch-bot/env
ExecStart=/usr/local/bin/aima-renew-watch-bot
Restart=always
RestartSec=10s

# hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/aima-renew-watch-bot
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
```

```sh
systemctl daemon-reload
systemctl enable --now aima-renew-watch-bot
systemctl status aima-renew-watch-bot
journalctl -u aima-renew-watch-bot -f
```

### 5. Обновление

```sh
GOOS=linux GOARCH=amd64 go build -o aima-renew-watch-bot ./cmd/bot
scp aima-renew-watch-bot user@server:/tmp/
ssh user@server '
  install -m 755 /tmp/aima-renew-watch-bot /usr/local/bin/ &&
  systemctl restart aima-renew-watch-bot
'
```

Миграции схемы выполняются при старте через `CREATE TABLE IF NOT
EXISTS`. Если в будущем появятся `ALTER TABLE` — этот же блок
выполнится автоматически.

### 6. Бэкапы

Бэкап `bot.db` без `ENC_KEY` бесполезен — URL'ы не расшифровать. Если
бэкапить вместе с ключом — теряется смысл шифрования at rest. По
умолчанию я ничего не бэкаплю: при потере БД пользователи переподпишутся
(данных, которые жалко терять, в БД нет).

Если всё же нужен снапшот — `sqlite3 bot.db ".backup '/path/backup.db'"`
работает без остановки сервиса (WAL-mode включён).

## Как работает мониторинг

- Scheduler-тик раз в минуту берёт **один** URL с самой давней
  проверкой и фетчит его, если age > 2 часа. Сериализованная очередь —
  AIMA не получает burst даже на больших объёмах подписок.
- При изменении статуса всем подписчикам этого URL (их может быть
  несколько — один URL дедуплицируется по нормализованному виду) уходит
  уведомление.
- При статусе 6 (Aprovado) подписка автоматически снимается, бот шлёт
  сводку всех переходов и удаляет данные.
- Если 5 фетчей подряд провалились (токен протух / редирект на login),
  подписка снимается с уведомлением.

## Лицензия

GPL-3.0
