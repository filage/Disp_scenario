import type { ActionEvent } from "@/features/events/types";
import { formatClock, formatIssueType } from "@/lib/display";

export function eventNarrative(event: ActionEvent) {
  const order = event.orderId || event.entityId;
  const issue = event.issueType ? formatIssueType(event.issueType) : "";
  const target = event.target || event.screen;
  const driver = payloadString(event, "driverName") || driverFromText(target);
  const value =
    payloadString(event, "value") ||
    payloadString(event, "newValue") ||
    payloadString(event, "time") ||
    payloadString(event, "pickupWindowEnd");

  switch (event.canonicalAction) {
    case "FILTER_ISSUES":
      return "Отфильтровал список по проблемным заказам.";
    case "INSPECT_ISSUE":
      return issue
        ? `Посмотрел проблему "${issue}"${order ? ` у заказа ${order}` : ""}.`
        : `Посмотрел индикатор проблемы${order ? ` у заказа ${order}` : ""}.`;
    case "OPEN_ORDER":
      return `Открыл заказ${order ? ` ${order}` : ""}.`;
    case "TAKE_ACTION":
      return `Открыл меню действий${order ? ` по заказу ${order}` : ""}.`;
    case "OPEN_DRIVER_ASSIGNMENT":
      return `Открыл окно отправки заказа водителям${order ? ` для заказа ${order}` : ""}.`;
    case "SELECT_DRIVER":
      return `Выбрал водителя${driver ? ` ${driver}` : ""}${order ? ` для заказа ${order}` : ""}.`;
    case "SEND_TO_SELECTED_DRIVER":
      return `Нажал "Send to Selected"${order ? ` для заказа ${order}` : ""}.`;
    case "ASSIGN_DRIVER":
      return `Назначил водителя${order ? ` для заказа ${order}` : ""}.`;
    case "OPEN_FIELD_EDITOR":
      return `Открыл редактирование поля${target ? ` "${target}"` : ""}${order ? ` в заказе ${order}` : ""}.`;
    case "CHANGE_FIELD_VALUE":
    case "EDIT_FIELD":
      return `Изменил значение поля${target ? ` "${target}"` : ""}${value ? ` на ${value}` : ""}${order ? ` в заказе ${order}` : ""}.`;
    case "SAVE":
      return `Сохранил изменения${order ? ` в заказе ${order}` : ""}.`;
    case "RESOLUTION_ATTEMPT":
      return `Попробовал закрыть проблему${issue ? ` "${issue}"` : ""}${order ? ` у заказа ${order}` : ""}.`;
    case "RESOLVE_ISSUE":
      return `Закрыл проблему${issue ? ` "${issue}"` : ""}${order ? ` у заказа ${order}` : ""}.`;
    case "MARK_PICKUP_COMPLETED":
      return `Отметил забор выполненным${order ? ` у заказа ${order}` : ""}.`;
    case "NAVIGATE":
      return `Перешел на экран "${event.screen || target}".`;
    default:
      return `Выполнил действие ${event.canonicalAction}${target ? `: ${target}` : ""}.`;
  }
}

export function timedEventNarrative(event: ActionEvent) {
  return `${formatClock(event.timestampMs)} - ${eventNarrative(event)}`;
}

function payloadString(event: ActionEvent, key: string) {
  const value = event.payload?.[key];
  return typeof value === "string" ? value.trim() : "";
}

function driverFromText(value: string) {
  const match = value.match(/(?:checkbox next to|driver checkbox)\s+([^,]+)/i);
  return match?.[1]?.trim() ?? "";
}
