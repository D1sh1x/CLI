
````markdown
# mygrep — распределённый `grep` на Go

Мини-утилита и микросервис для поиска по строкам локально и в распределённом режиме.
Поддерживает флаги, чтение из `stdin`/файла, параллелизм на горутинах и кворум при работе
через несколько узлов.

---

## Возможности

- Совместимые флаги с `grep`:
  - `-E` (regex, по умолчанию), `-F` (фиксированная строка)
  - `-i` (ignore case), `-v` (invert match)
  - `-m N` (max-count), `-c` (count only)
- Локальный режим: читает `stdin` или `-file`.
- Распределённый режим:
  - координатор делит вход на N шардов (self + peers)
  - рассылает задания по HTTP (`POST /v1/grep`)
  - агрегирует результат **по кворуму** (по умолчанию majority: `N/2+1`)
- Внутри каждого узла — пул воркеров на горутинах (через каналы).
- HTTP-сервер узла: `/health`, `/v1/grep`.

---

## Требования

- Go ≥ 1.24 (для локальной сборки).
- Для Docker-запуска — Docker/Compose.
- Открытые порты для узлов (по умолчанию `8080`, `8081`, …).

---

## Установка и сборка

```bash
git clone <https://github.com/D1sh1x/CLI> mygrep
cd mygrep
go build -o mygrep .
./mygrep help
````

---

## Быстрый старт (локально, без Docker)

### 1) Простой греп по файлу/входному потоку

```bash
# regex + ignore case
./mygrep -E -i -pattern "error|failed" -file examples/shardA.log

# только количество совпадений
./mygrep -c -pattern "ERROR" -file examples/shardA.log
```

### 2) Два узла + координатор на одной машине

Терминал A (узел A):

```bash
./mygrep serve -addr :8080 -node A
```

Терминал B (узел B):

```bash
./mygrep serve -addr :8081 -node B
```

Терминал C (координатор):

```bash
cat examples/shardA.log examples/shardB.log | \
./mygrep run -pattern "error|failed" -E -i \
  -peers http://localhost:8080,http://localhost:8081 \
  -show-node
```

Счёт:

```bash
cat examples/shardA.log examples/shardB.log | \
./mygrep run -c -pattern "error|failed" -E -i \
  -peers http://localhost:8080,http://localhost:8081
```

> **Замечание про кворум:**
> Узлов = self (координатор) + peers.
> По умолчанию `required = N/2+1`.
> Если узел один (`N=2`), по умолчанию нужен кворум 2 — при необходимости ослабь: `-quorum 1`.

---

## Запуск в Docker / Docker Compose

Сборка и запуск:

```bash
docker compose up --build -d nodeA nodeB
```

Проверки:

```bash
curl -s http://localhost:8080/health   # ok
curl -s http://localhost:8081/health   # ok
```

Одноразовый координатор внутри compose:

```bash
docker compose run --rm coordinator run \
  -pattern "error|failed" -E -i -show-node \
  -file /data/shardA.log \
  -peers http://nodeA:8080,http://nodeB:8081
```

> Внутри Compose обращайся к узлам по их **именам сервисов** (`nodeA`, `nodeB`), а не `localhost`.

---

## Прод: несколько серверов

### Вариант A — бинарь + systemd

На каждом узле:

```bash
scp mygrep user@node-1:/usr/local/bin/
ssh user@node-1 'sudo /usr/local/bin/mygrep serve -addr :8080 -node node-1'
```

(Рекомендуется оформить unit-файлом `systemd`, открыть 8080 в firewall/SG.)

Координатор:

```bash
cat all-logs.txt | ./mygrep run \
  -pattern "timeout|failed|error" -E -i \
  -peers http://node-1:8080,http://node-2:8080,http://node-3:8080 \
  -timeout 15s -show-node
```

### Вариант B — Docker на каждом узле

```bash
docker build -t mygrep:0.1.0 .
docker run -d --name mygrep-node --restart=always \
  -p 8080:8080 mygrep:0.1.0 \
  serve -addr :8080 -node $(hostname)
```

---

## CLI

```text
mygrep <cmd> [flags]

Команды:
  serve            — запустить HTTP-узел
  run              — локальный/распределённый запуск (если сразу флаги, run не обязателен)
  help | -h        — помощь

Общие флаги run:
  -pattern STR     — шаблон (обязателен; можно как позиционный аргумент)
  -file PATH       — читать из файла (по умолчанию stdin)
  -peers URLs      — список узлов через запятую (http://host:port,...)
  -quorum N        — размер кворума (по умолчанию majority)
  -workers N       — кол-во воркеров на узел (по умолчанию GOMAXPROCS)
  -timeout D       — общий таймаут (default 10s)
  -json            — вывод в JSON
  -show-node       — префикс строки id узла

Флаги grep:
  -E               — regex (по умолчанию)
  -F               — фиксированная строка
  -i               — ignore case
  -v               — invert match
  -m N             — максимум совпадений (на узел)
  -c               — печатать только количество
```

---

## HTTP API узла

* `GET /health` → `200 ok`
* `POST /v1/grep`

  * Request (JSON):

    ```json
    {
      "job_id": "job-1",
      "pattern": "ERROR",
      "flags": {"Regex": true, "IgnoreCase": true},
      "lines": ["foo", "ERROR: boom", "ok"],
      "workers": 4
    }
    ```
  * Response (JSON):

    ```json
    {
      "job_id": "job-1",
      "node": "A",
      "matches": ["ERROR: boom"],
      "count": 1,
      "error": ""
    }
    ```
  * Если `lines` не передавать и узел запущен с `-data-file`, он возьмёт строки из своего файла.

---

## Архитектура 

```
stdin/file → [координатор]
  ├─ шардирование строк на N частей (self + peers)
  ├─ параллельные HTTP-запросы на /v1/grep
  └─ сбор результатов до кворума → печать результата

[узел /v1/grep]
  ├─ очередь строк → канал in
  ├─ пул воркеров (N горутин) → тестер (regex/fixed, invert, count)
  └─ коллектор из out (строки + служебные COUNT) → ответ JSON
```

**Кворум:**
`N = 1 (self) + len(peers)`; по умолчанию `required = N/2 + 1`.
Как только достигли `required` успешных ответов — координатор печатает результат и завершает работу.

---

## Сравнение с GNU grep (ручная проверка)

Количество совпадений:

```bash
# GNU grep
grep -Ei 'error|failed' examples/shardA.log examples/shardB.log | wc -l

# mygrep
cat examples/shardA.log examples/shardB.log | \
./mygrep -c -E -i -pattern 'error|failed'
```

Числа должны совпасть.

---

## Примеры

```bash
# Локально, строки с указанием узла (self)
./mygrep -pattern 'payment_id=' -file examples/shardA.log -show-node

# Распределённо, JSON-вывод
cat examples/shardA.log examples/shardB.log | \
./mygrep run -pattern 'ERROR' -E -i \
  -peers http://localhost:8080,http://localhost:8081 \
  -json
```

## Структура проекта

```
mygrep/
├─ go.mod
├─ main.go            # CLI, парсинг флагов, команды run/serve
├─ version.go         # версия
├─ flags.go           # типы запрос/ответ/флаги
├─ grep.go            # доменная логика поиска, пул воркеров
├─ server.go          # HTTP-узел (/health, /v1/grep)
├─ coordinator.go     # шардирование, HTTP-раскидка, кворум, агрегация
├─ io.go              # чтение строк из stdin/файла
├─ util.go            # утилиты, лог-миддлвар, JSON-ответы
├─ Dockerfile
├─ docker-compose.yml
└─ examples/
   ├─ shardA.log
   └─ shardB.log
```

