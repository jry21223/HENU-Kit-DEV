from __future__ import annotations


def fen_to_yuan(price_fen: int) -> str:
    return f"{price_fen / 100:.1f}".rstrip("0").rstrip(".")


def format_package_summary(package: dict) -> str:
    includes = "、".join(package.get("includes", [])[:5])
    return (
        f"找到「{package['title']}」\n"
        f"适用：{package['school']} {package['college']} {package['grade']} {package['major']}\n"
        f"包含：{includes}\n"
        f"价格：{fen_to_yuan(int(package['priceFen']))} 元\n"
        f"购买请发送：!购买 {package['title'].replace('期末复习包', '').strip()}"
    )


def format_package_choices(packages: list[dict], command: str = "!购买") -> str:
    lines = ["找到多个课程包："]
    for index, package in enumerate(packages, start=1):
        lines.append(f"{index}. {package['grade']} {package['major']} {package['title']}")
    lines.append(f"请发送：{command} 1")
    return "\n".join(lines)


def format_order_payment(order: dict) -> str:
    return (
        f"订单已创建：{order['orderId']}\n"
        f"课程包：{order['title']}\n"
        f"金额：{fen_to_yuan(int(order['amountFen']))} 元\n"
        f"状态：等待微信扫码支付\n"
        f"二维码过期时间：{order['expiresAt']}"
    )


def format_recent_orders(orders: list[dict]) -> str:
    if not orders:
        return "当前聊天用户暂无订单。"
    lines = ["最近订单："]
    for order in orders:
        status = {
            "pending": "等待支付",
            "paying": "等待支付",
            "paid": "已支付",
            "delivered": "已发货",
            "expired": "已过期",
            "failed": "失败",
        }.get(order["status"], order["status"])
        lines.append(f"{order['title']}\n订单号：{order['orderId']}\n状态：{status}")
    return "\n\n".join(lines)


def help_text() -> str:
    return (
        "可用命令：\n"
        "!资料 <课程名>：查询课程复习包\n"
        "!购买 <课程名>：创建微信 Native mock 订单\n"
        "!订单：查询最近订单\n"
        "!重发 <订单号>：支付成功后重发下载入口\n"
        "!帮助：查看命令说明\n\n"
        "说明：支付确认和发货以后端状态为准；群聊不会发送 paid PDF。"
    )
