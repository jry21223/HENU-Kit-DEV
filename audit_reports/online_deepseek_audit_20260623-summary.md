# QuizCraft online bank DeepSeek audit summary

- Source JSONL: `audit_reports/online_deepseek_audit_20260623-full.jsonl`
- Priority CSV: `audit_reports/online_deepseek_audit_20260623-priority.csv`
- Total: 1513
- API errors: 0

## Status counts
- ok: 1433
- suspect: 68
- invalid: 12

## By bank
- history: ok=337, suspect=21, invalid=3
- java_programming: ok=15
- mayuan: ok=441, suspect=16
- sixiu: ok=283, suspect=10, invalid=7
- xigai: ok=357, suspect=21, invalid=2

## Issue types
- analysis_mismatch: 36
- answer_mismatch: 24
- stem_error: 18
- option_error: 9
- duplicate_option: 3
- typo: 2

## Top high-priority items
- history[269] invalid/high conf=1.0 ans=AB rec=AB issues=stem_error,option_error - 选项D末尾混入了下一题题干'6.近代中国反侵略战争失败的主要原因是（ ）。'，属于题干残留，导致选项错误。 | 近代中国，列强对华文化渗透主要表现在（ ）。
- history[304] suspect/high conf=1.0 ans=CD rec=CD issues=analysis_mismatch - 解析中称正确答案为A和D，但实际正确选项应为C和D，与当前答案CD矛盾。 | 新文化运动的主要阵地有（ ）。更多资料请加河南大学考试墙Q2139034270、河南大学小过儿QQ2221565155
- history[307] invalid/high conf=1.0 ans=ABC rec=AB issues=analysis_mismatch - 解析指出D选项不正确，但当前答案为ABC，包含D。解析与答案矛盾。 | 中华人民共和国的成立，标志着（ ）
- history[322] invalid/high conf=1.0 ans=ABCD rec=ABD issues=answer_mismatch,analysis_mismatch - 当前答案为ABCD，但解析明确指出C选项“推行预备立宪”不属于洋务运动，正确答案应为ABD，答案与解析矛盾。 | 从19世纪60年代到90年代，洋务派举办的洋务事业主要有（ ）
- sixiu[51] invalid/high conf=1.0 ans=错 rec=对 issues=analysis_mismatch - 解析明确说明人生价值是人生观的核心，与题干一致，但当前答案为'错'，解析与答案矛盾。 | 人生价值是人生观的核心。（	）
- sixiu[108] invalid/high conf=1.0 ans=B rec=A issues=answer_mismatch,analysis_mismatch - 题干问的是社会层面的价值追求，社会主义核心价值观社会层面是自由、平等、公正、法治，但解析说正确答案是A（国家层面），且当前答案B与解析矛盾。 | 回答我们要建设什么样的社会的重大问题,与实现国家治理体系和治理能力现代化的要求相契合,揭示社会主义社会发展的价 值取向,这一价值追求是()。
- sixiu[110] invalid/high conf=1.0 ans=C rec=C issues=option_error,duplicate_option - 选项A和B内容完全相同，均为'理论认同'，存在重复选项，导致选项缺失一个有效选项。 | 习近平说：“我国是一个有着13亿多人口、56个民族的大国,确立反映全国各族人民共同认同的价值观‘最大公约数’,使 全体人民同心同德、团结奋进,关乎国家前途命运,关乎人民幸福安康。”。其中的“最大公约数”是指（）
- sixiu[202] invalid/high conf=1.0 ans=BCD rec=ABC issues=answer_mismatch,analysis_mismatch - 当前答案BCD与解析中给出的正确答案ABC不一致，且解析明确指出稳定性并非鲜明特点，因此答案应为ABC。 | 下列哪些是个人品德具有的鲜明特点（）
- sixiu[211] invalid/high conf=1.0 ans=ABC rec=C issues=answer_mismatch,analysis_mismatch - 题干问的是'最简单、最起码的公共道德准则'，应选社会公德。当前答案ABC错误，解析也错误，将职业道德和家庭道德归为社会公德的具体体现，但职业道德和家庭道德并非公共道德准则。 | 人们在社会生活中形成和应当遵守的最简单、最起码的公共道德准则是()
- sixiu[260] invalid/high conf=1.0 ans=C rec=A issues=answer_mismatch,analysis_mismatch - 题干强调法律对所有人平等适用，对应普遍适用性；当前答案C与解析矛盾，解析指出正确答案为A。 | 任何人不论权力大小、职位高低,只要有违法犯罪行为,就要依法追究和承担法律责任。这体现的是法律的（）
- sixiu[264] invalid/high conf=1.0 ans=C rec=D issues=answer_mismatch,analysis_mismatch - 题干问权利保障的关键环节，解析明确指出司法保障是关键环节，但当前答案选C（行政保护），与解析矛盾。 | 权利保障是法治思维的基本内容之一。权利的法律保障包括四方面的内容,其中,权利保障的关键环节是（）
- xigai[221] invalid/high conf=1.0 ans=错 rec=对 issues=analysis_mismatch - 题干完整表述了'人民是历史的创造者，是决定党和国家前途命运的根本力量'，与官方表述一致，应判断为正确。但解析称题干缺少关键部分，与事实不符，导致答案与解析矛盾。 | 人民是历史的创造者，是决定党和国家前途命运的根本力量。（ ）
- xigai[302] invalid/high conf=1.0 ans=对 rec=错 issues=analysis_mismatch,answer_mismatch - 解析明确指出题干中'组织路线'应为'群众路线'，因此原题表述错误，应判'错'，但当前答案为'对'，答案与解析矛盾。 | 组织路线始终是党的生命线和根本工作路线，是我们党永葆青春活力和战斗力的重要传家宝。（ ）
- mayuan[411] suspect/high conf=0.95 ans=对 rec=错 issues=analysis_mismatch - 题干中“反映人与人之间的物质关系”表述错误，使用价值应反映人与物的关系。解析指出错误，但当前答案却为“对”，答案与解析矛盾。 | 使用价值是指商品能够满足人的某种需要的有用性，反映的是人与人之间的物质关系，是商品的自然属性。
- sixiu[42] suspect/high conf=0.95 ans=D rec=ABC issues=answer_mismatch,analysis_mismatch - 题干问人生目的是核心的原因，正确选项应为A、B、C，但当前答案选D，且解析指出D错误，说明答案与解析矛盾。 | 人生目的是人生观的核心，这是因为（	）
- sixiu[140] suspect/high conf=0.95 ans=B rec=A issues=answer_mismatch,analysis_mismatch - 当前答案B与解析中明确指出的正确答案A矛盾，解析内容支持A选项。 | 马克思主义道德观认为,道德在本质上是（）的特殊调节方式。
- xigai[290] suspect/high conf=0.95 ans=B rec=B issues=option_error - 选项A、B、C、D中均出现'对本'，应为'基本'，属于严重错别字。题干和答案逻辑正确，但选项错误影响理解。 | （ ）是坚持和发展中国特色社会主义的行动指南。
- history[34] suspect/medium conf=1.0 ans=BCD rec=ACD issues=answer_mismatch - 三大法宝是统一战线、武装斗争和党的建设，对应选项A、C、D。当前答案BCD包含B群总路线（应为群众路线），且缺少A统一战线，与解析矛盾。 | 中国共产党在中国革命中战胜敌人的三个法宝是（ ）。
- history[72] suspect/medium conf=1.0 ans=B rec=B issues=duplicate_option - 选项C和D均为'党的七大'，重复。应修改其中一个为正确选项，如'六届六中全会'已正确，但选项重复需修正。 | 1938年，毛泽东在（ ）上明确提出了“马克思主义中国化”这个命题。
- xigai[372] suspect/medium conf=1.0 ans=C rec=C issues=duplicate_option - 选项C和D内容完全相同，均为'军种主建'，属于重复选项。 | 国防和军队改革取得历史性突破，形成军委管总、战区主战、（ ）新格局。
- history[90] suspect/medium conf=0.95 ans=ABC rec=ABC issues=stem_error - 题干末尾包含广告信息'更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿Q92221565155'，属于无关文本，应删除。 | 抗日民族统一战线的策略总方针是（ ）。更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿Q92221565155
- history[97] suspect/medium conf=0.95 ans=D rec=D issues=analysis_mismatch - 解析中称选项B准确概括了核心思想，但当前答案为D，解析与答案矛盾。 | 洋务运动的指导思想是（ ）
- history[183] suspect/medium conf=0.95 ans=AB rec=AB issues=analysis_mismatch - 解析中正确指出基本口号是民主与科学，对应选项A和B，但解析末尾却错误地写成了A和D，与当前答案AB不一致。 | 新文化运动的基本口号是（ ）。
- history[291] suspect/medium conf=0.95 ans=BC rec=BCD issues=answer_mismatch - 解析明确指出《新民主主义论》也是核心文献，但当前答案BC未包含D，与解析矛盾。 | 在20世纪30年代后期和40年代前期，毛泽东论述新民主主义革命理论的著作是（ ）
- xigai[101] suspect/medium conf=0.95 ans=ABC rec=ABCD issues=answer_mismatch - 题干为'必须坚持教育（ ）'，选项D'为改革开放和社会主义现代化建设服务'是教育方针的重要组成部分，常见表述为'四为服务'，应全选。当前答案ABC遗漏D，解析也未提及D，与标准表述不符。 | 必须坚持教育（ ）。
- history[39] suspect/medium conf=0.9 ans=A rec=None issues=analysis_mismatch - 解析中讨论的选项A内容为'五四运动是资产阶级领导的民主革命运动'，但题干中选项A实际为'是中共领导的彻底的反帝反封建革命运动'，两者不一致，解析与当前答案不匹配。 | 下列关于五四运动的评价，不确切的是（ ）
- history[40] suspect/medium conf=0.9 ans=C rec=None issues=analysis_mismatch - 解析中称D是正确答案，但当前答案为C，且解析内容与答案矛盾。 | 在中国近代史上规定允许外国人在中国办工厂的条约是（ ）。
- history[57] suspect/medium conf=0.9 ans=ABCD rec=ABCD issues=analysis_mismatch - 解析中提到了《北京条约》割让九龙司，但选项中没有《北京条约》，且解析未提及《改定伊犁条约》割地，而答案包含D。解析与答案不完全一致。 | 下列条约中涉及到割地的条约是（ ）
- history[94] suspect/medium conf=0.9 ans=AD rec=AD issues=stem_error - 题干末尾的选项D中包含了无关文本'2.近代中国的历史任务是（ ）'，属于题干残留广告或无关文本，应删除。 | 近代中国占支配地位的主要社会矛盾是（ ）
- history[170] suspect/medium conf=0.9 ans=B rec=None issues=stem_error - 题干末尾包含广告信息'更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿QQ2221565155'，属于无关文本，应删除。 | 中国革命历史上制定的第一个土地法是（ ）更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿QQ2221565155
- history[192] suspect/medium conf=0.9 ans=C rec=None issues=stem_error - 题干末尾包含广告信息'更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿QQ2221565155'，属于无关文本，应删除。 | 孙中山三民主义思想的核心是（ ）。更多资料请加河南大学考试墙QQ2139034270、河南大学小过儿QQ2221565155
- history[214] suspect/medium conf=0.9 ans=ABD rec=ABD issues=analysis_mismatch - 解析中称C项为大地主大资产阶级，但选项C是城市小资产阶级，解析内容与选项不符，存在明显错误。 | 在抗日民族统一战线中，中国共产党争取的中间势力指的是（ ）
- history[217] suspect/medium conf=0.9 ans=C rec=None issues=stem_error - 题干中'天平天国'应为'太平天国'，属于严重错别字，影响题目准确性。 | 以下不属于天平天国与旧式的农民战争相比新特点的是（ ）
- history[286] suspect/medium conf=0.9 ans=ABCD rec=ABCD issues=analysis_mismatch - 解析中选项C为'同农民结成了联盟'，但题干选项C是'建立了中华民国'，解析与选项不一致。 | 辛亥革命在比较完全意义上开始了中国资产阶级民主革命，是因为资产阶级革命派（ ）
- history[305] suspect/medium conf=0.9 ans=B rec=B issues=analysis_mismatch - 解析中正确答案B的表述为“说明了太平天国运动爆发的原因”，但选项B原文是“列强侵略激化了国内已尖锐的阶级矛盾”，两者不一致，解析未准确对应选项内容。 | “太平天国运动是鸦片战争炮声的回声”，这句话反映了（ ）
- mayuan[171] suspect/medium conf=0.9 ans=A rec=A issues=option_error - 选项D末尾包含无关文本'22.生产关系是人们在'，疑似题干残留或格式错误。 | 习近平强调，“ 历史是最好的教科书”，“历史的经验值得注意，历史的教训更应引以为戒”，“中国革命历史是最好的营养剂”。人们能够从历史中 汲取经 验教训，是因为( )
- mayuan[205] suspect/medium conf=0.9 ans=B rec=A issues=answer_mismatch - 题干问形成商品价值量的劳动尺度，正确应为社会必要劳动时间，选项A正确。当前答案B（简单劳动）是衡量社会必要劳动时间的尺度，但题干直接问形成价值量的劳动尺度，应选A。解析也矛盾，说正确答案是B但解释中又提到社会必要劳动时间。 | 形成商品价值量的劳动，是以（ ）
- mayuan[251] suspect/medium conf=0.9 ans=AB rec=AB issues=stem_error - 题干末尾出现无关文本'河南大学小过儿 QQ2221565155'，疑似广告或水印残留，应删除。 | 劳动力商品使用价值的特殊性表现在（ ）、河南大学小过儿 QQ2221565155
- mayuan[404] suspect/medium conf=0.9 ans=错 rec=对 issues=analysis_mismatch - 题干表述正确：社会意识相对独立性的突出表现就是与社会存在发展的不完全同步性和不平衡性。解析却认为混淆了‘突出表现’与‘原因’，但题干并未提及‘原因’，解析有误。 | 社会意识与社会存在发展具有不完全同步性和不平衡性，是社会意识相对独立性的突出表现。
- mayuan[409] suspect/medium conf=0.9 ans=对 rec=对 issues=stem_error - 题干不完整，结尾为'挫折简单'，缺少后续内容，可能影响判断。但根据现有部分和解析，答案'对'合理。 | 习近平在纪念毛泽东同志诞辰 120 周年座谈会上的讲话中指出:“不能把历史顺境中的成功简单归功于个人，也不能把历史逆境中的挫折简单
