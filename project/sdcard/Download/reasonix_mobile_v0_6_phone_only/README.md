# Reasonix Mobile v0.6 PHONE-ONLY

Собственная from-scratch архитектура Reasonix для Android/Realme Note 60. Готовые LLM и чужие веса не являются основой проекта.

## Что нового в v0.6

- новый project-owned формат `RXM6MMAP`;
- POSIX `mmap` loader: веса читаются из файла без второй полной копии в heap;
- постоянный INT8 matvec scratch buffer вместо новой scratch-аллокации на каждый Linear;
- mixed storage: критичные матрицы INT8, expert MLP может храниться group-wise INT4 (group=32);
- INT4 не включается вслепую: phone script сравнивает INT8 и mixed и выбирает mixed только если он меньше и сохраняет >=85% INT8 deep throughput;
- ring K/V cache из v0.5 сохранён;
- один phone-only Termux скрипт, умеющий переносить проект из Android shared storage в Termux HOME, где нативные бинарники можно исполнять;
- mobile-scale BENCH-ONLY fixture на конфигурации `mobile_s` (23,574,210 параметров) для честного измерения полноразмерного графа на телефоне.

## Проверено в текущей среде

- Python regression: 23/23 PASS.
- Native C++ INT4 primitive hand-check: exact PASS.
- Python quant-reference <-> C++ RXM6 runtime: FAST/STANDARD/DEEP PASS для INT8 и mixed.
- Максимальная ошибка логитов в correctness fixture: < 2.5e-7.
- Training smoke from random init: loss 5.6031 -> 2.8980 за 30 шагов.
- `mobile_s` BENCH-ONLY RXM6: 18,188,352 bytes, успешно загружается mmap и выполняет полный граф.

Host benchmark НЕ является результатом Realme Note 60. На host для `mobile_s` BENCH-ONLY было примерно 345/224/170 token-steps/s (FAST/STANDARD/DEEP), max RSS около 22 MB. Эти числа нужны только для проверки runtime.

Текущий mixed INT4 также НЕ объявлен ускорением. На host smoke-файле он был на 11.95% меньше INT8, но deep throughput составил только ~60.5% от INT8. Поэтому phone autotuner имеет право оставить INT8.

## Запуск только на телефоне

См. `PHONE_START.txt`.

После распаковки в Termux:

```bash
bash scripts/phone_one_tap.sh
```

Скрипт сам соберёт нативный runtime, выполнит correctness gate, сравнит INT8/mixed, прогонит mobile-scale граф, выведет RAM/thermal и закончит строкой:

`REASONIX_V06_PHONE_ONLY_PASS`

## Честные ограничения

- `mobile_s_v06_mixed_BENCH_ONLY.rxm6` содержит случайные веса. Это НЕ обученный ассистент и не доказательство интеллекта.
- Полноценное обучение большой Reasonix только на Realme ещё не реализовано в native C++.
- INT4 kernel v0.6 correctness-first и пока не ARM-оптимизирован настолько, чтобы гарантированно обгонять INT8.
- Реальные tokens/s, RSS и thermal для Realme появляются только после запуска phone script на физическом телефоне.
