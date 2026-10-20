from pathlib import Path

from PIL import Image
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.cidfonts import UnicodeCIDFont
from reportlab.pdfgen import canvas

from scripts.h3cne_pdf_source import convert_h3cne_pdf


COLORS = {"black": (0, 0, 0), "red": (1, 0, 0), "blue": (0, 0, 1)}
pdfmetrics.registerFont(UnicodeCIDFont("STSong-Light"))


def write_source(
    path: Path,
    pages: list[list[list[tuple[str, str]]]],
    page_images: dict[int, Path] | None = None,
    line_spacing: int = 22,
) -> None:
    document = canvas.Canvas(str(path), pagesize=(595, 842))
    for page_number, lines in enumerate(pages, start=1):
        for index, fragments in enumerate(lines):
            position = 56
            for color, text in fragments:
                font = "STSong-Light" if any(ord(char) > 127 for char in text) else "Helvetica"
                document.setFillColorRGB(*COLORS[color])
                document.setFont(font, 12)
                document.drawString(position, 760 - index * line_spacing, text)
                position += document.stringWidth(text, font, 12)
        if page_images and page_number in page_images:
            document.drawImage(str(page_images[page_number]), 56, 650, width=100, height=30)
        document.showPage()
    document.save()


def test_converts_red_multi_answer_without_splitting_an_ip_address(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(
        source,
        [[
            [("black", "1. Host 192.168.1.1 can use __"), ("red", "AC"), ("black", "__. ")],
            [("black", "A. Static route B. HTTP C. RIP")],
            [("blue", "* Static routing and RIP provide routes.")],
        ]],
    )

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert len(result["source_questions"]) == 1
    question = result["bank"]["questions"][0]
    assert question["type"] == "multi"
    assert question["answer"] == [0, 2]
    assert question["content"] == "Host 192.168.1.1 can use ____."
    assert question["options"] == ["Static route", "HTTP", "RIP"]
    assert question["analysis"] == "Static routing and RIP provide routes."
    record = result["source_questions"][0]
    assert record["source_question_id"] == "gb0192_main_q0001"
    assert record["source_pages"] == [1]
    assert record["source_answer"] == "AC"
    assert record["answer_spans"][0]["page"] == 1
    assert result["quarantine"] == []


def test_preserves_an_inline_ip_address_as_a_blank_answer(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(
        source,
        [[
            [("black", "1. The broadcast address is __"), ("red", "202.135.111.255"), ("black", "__.")],
            [("blue", "* The final octet is the host broadcast value.")],
        ]],
    )

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    question = result["bank"]["questions"][0]
    assert question["type"] == "blank"
    assert question["answer"] == "202.135.111.255"
    assert question["options"] is None
    assert "202.135.111.255" not in question["content"]


def test_keeps_cross_page_commands_and_excludes_the_closing_essay(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(
        source,
        [
            [
                [("black", "1. Set the route with __"), ("red", "B"), ("black", "__.")],
                [("black", "192.168.1.0/24 via 10.1.1.1")],
            ],
            [
                [("black", "A. reset B. ip route-static")],
                [("blue", "* The command creates a static route.")],
            ],
            [[("black", "H3C网络学院")], [("black", "网 络")], [("black", "Closing essay outside the question bank.")]],
        ],
    )

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    question = result["bank"]["questions"][0]
    assert question["answer"] == 1
    assert question["options"] == ["reset", "ip route-static"]
    assert "\n192.168.1.0/24 via 10.1.1.1" in question["content"]
    assert result["source_questions"][0]["source_pages"] == [1, 2]
    assert "Closing essay" not in question["content"]


def test_preserves_image_evidence_but_quarantines_unrenderable_and_missing_figures(tmp_path):
    source = tmp_path / "h3cne.pdf"
    figure = tmp_path / "topology.png"
    Image.new("RGB", (100, 30), "green").save(figure)
    write_source(
        source,
        [
            [
                [("black", "1. 图示中的根桥是____。"), ("red", "A")],
                [("black", "A. SWA B. SWB")],
                [("blue", "* SWA is the root in the figure.")],
            ],
            [
                [("black", "2. 如图所示，阻塞端口是____。"), ("red", "B")],
                [("black", "A. P1 B. P2")],
            ],
        ],
        page_images={1: figure},
    )
    destination = tmp_path / "output"

    result = convert_h3cne_pdf(source, destination, expected_count=2)

    assert len(result["source_questions"]) == 2
    first_image = result["source_questions"][0]["images"][0]
    assert first_image["page"] == 1
    assert first_image["bbox"] == [56.0, 162.0, 156.0, 192.0]
    assert (destination / first_image["asset_path"]).is_file()
    assert first_image["asset_path"] in (destination / "review.md").read_text()
    assert result["bank"]["questions"] == []
    reasons = {item["source_question_id"]: item["reasons"] for item in result["quarantine"]}
    assert "image_requires_rendering" in reasons["gb0192_main_q0001"]
    assert "missing_source_image" in reasons["gb0192_main_q0002"]


def test_quarantines_missing_red_answers_without_guessing_from_the_explanation(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. Which route should be used?")],
        [("black", "A. Static B. RIP")],
        [("blue", "* The correct answer is A.")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert result["source_questions"][0]["source_answer"] == ""
    assert result["bank"]["questions"] == []
    assert "missing_source_answer" in result["quarantine"][0]["reasons"]


def test_preserves_g_and_h_choices_without_truncating_the_source_answer(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. Select the two final protocols: __"), ("red", "GH"), ("black", "__.")],
        [("black", "A. FTP B. HTTP C. RIP D. OSPF")],
        [("black", "E. BGP F. EIGRP G. PIM H. IGMP")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert result["bank"]["questions"][0]["answer"] == [6, 7]
    assert result["bank"]["questions"][0]["options"][-2:] == ["PIM", "IGMP"]
    assert result["source_questions"][0]["source_answer"] == "GH"


def test_quarantines_an_unreadable_red_answer_even_when_blue_text_suggests_a_choice(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. Select the default route: __"), ("red", "?"), ("black", "__.")],
        [("black", "A. route-static B. no route")],
        [("blue", "* 正确答案为A。")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert result["bank"]["questions"] == []
    assert "invalid_source_answer" in result["quarantine"][0]["reasons"]
    assert result["source_questions"][0]["source_answer"] == "?"


def test_red_emphasis_inside_blue_commentary_is_not_an_additional_answer(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. Select the boot command: __"), ("red", "B"), ("black", "__.")],
        [("black", "A. startup B. boot-loader")],
        [("blue", "* startup selects the "), ("red", "configuration"), ("blue", " file.")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert result["source_questions"][0]["source_answer"] == "B"
    assert result["bank"]["questions"][0]["analysis"] == "startup selects the configuration file."
    assert result["bank"]["questions"][0]["answer"] == 1


def test_keeps_tightly_spaced_source_lines_in_their_correct_colour_roles(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. Select __"), ("red", "A"), ("black", "__.")],
        [("black", "A. route-static B. no route")],
        [("blue", "* Static routes use route-static.")],
    ]], line_spacing=12)

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    question = result["bank"]["questions"][0]
    assert question["answer"] == 0
    assert question["options"] == ["route-static", "no route"]
    assert question["analysis"] == "Static routes use route-static."


def test_keeps_option_columns_when_the_pdf_omits_spaces_between_labels(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("black", "1. The acknowledgement field is __"), ("red", "D"), ("black", "__.")],
        [("black", "A. aB. bC. a+1D. b+1")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    assert result["bank"]["questions"][0]["options"] == ["a", "b", "a+1", "b+1"]
    assert result["bank"]["questions"][0]["answer"] == 3


def test_retains_a_red_question_marker_without_treating_it_as_the_answer(tmp_path):
    source = tmp_path / "h3cne.pdf"
    write_source(source, [[
        [("red", "?"), ("black", "1. Choose __"), ("red", "B"), ("black", "__.")],
        [("black", "A. RIP B. OSPF")],
    ]])

    result = convert_h3cne_pdf(source, tmp_path / "output", expected_count=1)

    record = result["source_questions"][0]
    assert record["source_answer"] == "B"
    assert record["answer_spans"][0]["bbox"][0] > 100
    assert record["annotations"][0]["text"] == "?"
    assert result["bank"]["questions"][0]["answer"] == 1
