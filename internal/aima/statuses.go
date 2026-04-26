package aima

// StatusLabels — словарь статусов AIMA Renovação. Дублирует
// STATUS_LABELS из userscript portal-renovacoes_aima.user.js. При
// добавлении нового статуса синхронизируй вручную в обоих местах.
//
// Источники: https://t.me/aimairn/43114/134298, https://t.me/aimairn/43114/136559
var StatusLabels = map[int]string{
	1:  "Регистрация",
	5:  "Заявка передана сотруднику",
	11: "Внутренняя проверка",
	14: "Внутренняя проверка",
	15: "Финальный анализ",
	20: "?",
	6:  "Одобрение",
}

// StatusFlow — типичная последовательность.
var StatusFlow = []int{1, 5, 11, 14, 15, 20, 6}

// StatusApproved — терминальный статус, при достижении бот
// отписывает пользователя и удаляет данные.
const StatusApproved = 6

func Label(status int) string {
	if l, ok := StatusLabels[status]; ok {
		return l
	}
	return "Неизвестный статус"
}
