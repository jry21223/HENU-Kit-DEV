import pytest

from scripts.h3c_pdf_workflow import convert_h3c_pdf, parse_h3c_text


def test_se_numbering_restart_preserves_distinct_questions_and_ip_addresses():
    result = parse_h3c_text(
        """考试介绍
问题 1
★
配置地址10.1.1.5时使用哪个命令？
A. ip address 10.1.1.5 24
B. display ip interface
正确答案: A
章节: (无)
Explanation
说明/参考:
ip address命令用于配置接口地址。
问题 1
另一个独立的问题？
A. 第一个选项
B. 第二个选项
正确答案: B
章节: (无)
Explanation
说明/参考:
第二个选项描述了该行为。
""",
        source_file="H3CSE RS+_372.pdf",
    )

    assert [q["id"] for q in result["source_questions"]] == [
        "gb0372_main_s01_q0001",
        "gb0372_main_s02_q0001",
    ]
    assert result["questions"][0]["options"] == [
        "ip address 10.1.1.5 24",
        "display ip interface",
    ]
    assert result["questions"][1]["answer"] == 1


def test_figure_is_preserved_for_review_and_not_published_as_plain_text():
    result = parse_h3c_text(
        """[H3C_PAGE:5]
问题 1
[H3C_IMAGE:p5-i1]
根据网络拓扑选择路径。
A. R1-R2
B. R1-R3
正确答案: A
说明/参考:
依据所示链路的开销计算。
""",
        source_file="H3CSE RS+_382.pdf",
    )

    assert result["questions"] == []
    assert result["source_questions"][0]["source"]["images"] == ["p5-i1"]
    assert "H3C_IMAGE" not in result["source_questions"][0]["content"]
    assert result["review"][0]["reasons"] == ["figure_required"]


def test_pdf_conversion_requires_the_reviewed_source_count(tmp_path):
    import pymupdf

    source = tmp_path / "GB0-330_202410.pdf"
    document = pymupdf.open()
    page = document.new_page()
    page.insert_text((50, 50), "1. Pick a command\nA. display ip interface\nB. reset\nAnswer: A")
    document.save(source)
    document.close()

    with pytest.raises(ValueError, match="expected 2 source questions, found 1"):
        convert_h3c_pdf(source, tmp_path / "output", expected_count=2)
    assert not (tmp_path / "output" / "bank.json").exists()

    result = convert_h3c_pdf(source, tmp_path / "output", expected_count=1)
    assert result["source_questions"][0]["source"]["pages"] == [1]
    assert result["source_questions"][0]["source"]["regions"]
    assert result["questions"][0]["answer"] == 0
    assert (tmp_path / "output" / "source_questions.json").exists()


def test_numbered_source_does_not_split_an_ip_at_the_start_of_a_line():
    result = parse_h3c_text(
        """1. Select the configured interface address:
10.0.0.1 255.255.255.0
2.2 2.1/23
2002::1/64
A. interface A
B. interface B
Answer: A
2. 10G is a valid question opening
A. one
B. two
Answer: B
""",
        source_file="新增GB0-192（20240707）（52题）.pdf",
    )
    assert len(result["source_questions"]) == 2
    assert "10.0.0.1 255.255.255.0" in result["questions"][0]["content"]


def test_unnumbered_aaa_item_is_split_from_question_17_and_kept_for_review():
    result = parse_h3c_text(
        """[H3C_PAGE:4]
17.PIM neighbor message maps to a multicast MAC address.
Answer: 01-00-5E-00-00-0D
AAA 作为一种网络安全管理机制，可以通过如下哪些协议来实现?
A. TACACS+
C B.EAPOL
c C.802.1X
C D.RADIUS+
Answer：A
18.SSH encrypts which content?
A. only the password
B. the whole session
Answer：B
""",
        source_file="新增GB0-372（20240719）（41题）.pdf",
    )

    assert [q["id"] for q in result["source_questions"]] == [
        "gb0372_supp20240719_s01_q0017",
        "gb0372_supp20240719_s01_u01_aaa",
        "gb0372_supp20240719_s01_q0018",
    ]
    aaa = result["source_questions"][1]
    assert "AAA 作为一种网络安全管理机制" in aaa["content"]
    assert aaa["source_answer"] == "A"
    assert "incomplete_options" in aaa["review_reasons"] or "missing_options" in aaa["review_reasons"]
    assert "AAA 作为" not in result["source_questions"][0]["raw_text"]
