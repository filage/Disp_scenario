const actionDescriptions: Record<string, string> = {
  NAVIGATE: "Перешёл на другой экран",
  FILTER_ISSUES: "Отфильтровал проблемы",
  INSPECT_ISSUE: "Проверил проблему заказа",
  OPEN_ORDER: "Открыл заказ",
  TAKE_ACTION: "Открыл меню действий",
  CHECK: "Проверил состояние заказа",
  OPEN_DRIVER_ASSIGNMENT: "Открыл выбор курьера",
  SELECT_DRIVER: "Выбрал курьера",
  SEND_TO_SELECTED_DRIVER: "Отправил заказ курьеру",
  ASSIGN_DRIVER: "Назначил курьера",
  MARK_PICKUP_COMPLETED: "Отметил забор выполненным",
  OPEN_FIELD_EDITOR: "Открыл редактор поля",
  CHANGE_FIELD_VALUE: "Изменил значение поля",
  EDIT_FIELD: "Отредактировал поле",
  SAVE: "Сохранил изменения",
  RESOLUTION_ATTEMPT: "Попытался решить проблему",
  RESOLVE_ISSUE: "Решил проблему заказа",
};

export function actionDescription(action: string) {
  return actionDescriptions[action] ?? "Другое действие сценария";
}
