#!/usr/bin/env python3
# -*- coding: UTF-8 -*
"""
LLM 服务 - 高并发版本
支持异步并发、多 Key 轮询、自动重试
"""

import os
import asyncio
import re
from typing import List, Dict, Optional, Callable, Awaitable, Tuple
from dataclasses import dataclass
import httpx


@dataclass
class LLMConfig:
    """LLM 配置"""
    provider: str
    api_keys: List[str]  # 支持多个 API Key
    base_url: Optional[str] = None
    model: Optional[str] = None
    max_concurrent: int = 5  # 最大并发数
    timeout: float = 30.0    # 超时时间
    max_retries: int = 3     # 最大重试次数


class LLMProvider:
    """LLM 提供商基类"""
    
    def __init__(self, config: LLMConfig):
        self.config = config
        self.key_index = 0  # 当前使用的 key 索引
        self.client = httpx.AsyncClient(timeout=self.config.timeout)

    async def aclose(self) -> None:
        await self.client.aclose()
    
    def get_next_key(self) -> str:
        """轮询获取下一个 API Key"""
        key = self.config.api_keys[self.key_index % len(self.config.api_keys)]
        self.key_index += 1
        return key
    
    async def generate_with_retry(
        self, 
        prompt: str, 
        model: Optional[str] = None,
        retries: int = 0
    ) -> str:
        """带重试的生成"""
        try:
            return await self.generate(prompt, model)
        except Exception as e:
            if retries < self.config.max_retries:
                # 指数退避
                wait_time = 2 ** retries
                print(f"请求失败，{wait_time}s 后重试... ({retries+1}/{self.config.max_retries})")
                await asyncio.sleep(wait_time)
                return await self.generate_with_retry(prompt, model, retries + 1)
            raise e
    
    async def generate(self, prompt: str, model: Optional[str] = None) -> str:
        raise NotImplementedError


class OpenAIProvider(LLMProvider):
    """OpenAI / Azure OpenAI"""
    
    async def generate(self, prompt: str, model: Optional[str] = None) -> str:
        model = model or self.config.model or "gpt-3.5-turbo"
        api_key = self.get_next_key()
        base_url = self.config.base_url or "https://api.openai.com/v1"
        
        response = await self.client.post(
            f"{base_url}/chat/completions",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            json={
                "model": model,
                "messages": [
                    {"role": "system", "content": "你是一个教育助手，专门为学生生成考试题目的解析。解析要简洁明了，指出正确答案并解释原因。"},
                    {"role": "user", "content": prompt}
                ],
                "temperature": 0.7,
                "max_tokens": 500,
            }
        )
        
        if response.status_code == 429:  # 速率限制
            raise Exception("Rate limit exceeded")
        if response.status_code != 200:
            raise Exception(f"OpenAI API error: {response.text}")
        
        data = response.json()
        return data["choices"][0]["message"]["content"]


class DeepSeekProvider(LLMProvider):
    """DeepSeek (便宜好用，推荐)"""
    
    async def generate(self, prompt: str, model: Optional[str] = None) -> str:
        model = model or self.config.model or "deepseek-chat"
        api_key = self.get_next_key()
        base_url = self.config.base_url or "https://api.deepseek.com/v1"
        
        response = await self.client.post(
            f"{base_url}/chat/completions",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            json={
                "model": model,
                "messages": [
                    {"role": "system", "content": "你是一个教育助手，专门为学生生成考试题目的解析。解析要简洁明了，指出正确答案并解释原因。"},
                    {"role": "user", "content": prompt}
                ],
                "temperature": 0.7,
                "max_tokens": 500,
            }
        )
        
        if response.status_code == 429:
            raise Exception("Rate limit exceeded")
        if response.status_code != 200:
            raise Exception(f"DeepSeek API error: {response.text}")
        
        data = response.json()
        return data["choices"][0]["message"]["content"]


class SiliconFlowProvider(LLMProvider):
    """SiliconFlow (国内，免费额度多)"""
    
    async def generate(self, prompt: str, model: Optional[str] = None) -> str:
        model = model or self.config.model or "Qwen/Qwen2.5-7B-Instruct"
        api_key = self.get_next_key()
        base_url = self.config.base_url or "https://api.siliconflow.cn/v1"
        
        response = await self.client.post(
            f"{base_url}/chat/completions",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            json={
                "model": model,
                "messages": [
                    {"role": "system", "content": "你是一个教育助手，专门为学生生成考试题目的解析。解析要简洁明了，指出正确答案并解释原因。"},
                    {"role": "user", "content": prompt}
                ],
                "temperature": 0.7,
                "max_tokens": 500,
            }
        )
        
        if response.status_code == 429:
            raise Exception("Rate limit exceeded")
        if response.status_code != 200:
            raise Exception(f"SiliconFlow API error: {response.text}")
        
        data = response.json()
        return data["choices"][0]["message"]["content"]


class LLMService:
    """LLM 服务管理 - 高并发版本"""
    
    PROVIDERS = {
        "openai": OpenAIProvider,
        "deepseek": DeepSeekProvider,
        "siliconflow": SiliconFlowProvider,
    }
    
    @classmethod
    def create(cls, config: LLMConfig) -> LLMProvider:
        if config.provider not in cls.PROVIDERS:
            raise ValueError(f"不支持的提供商: {config.provider}")
        return cls.PROVIDERS[config.provider](config)

    @staticmethod
    async def close_provider(provider: LLMProvider) -> None:
        await provider.aclose()

    @staticmethod
    async def close_providers(providers: List[LLMProvider]) -> None:
        await asyncio.gather(*(p.aclose() for p in providers), return_exceptions=True)
    
    @staticmethod
    def build_prompt(question: Dict) -> str:
        """构建生成解析的 prompt"""
        q_type = question.get("type", "single")
        content = question.get("content", "")
        options = question.get("options", [])
        answer = question.get("answer", "")
        global_context = str(
            question.get("global_context")
            or question.get("full_context")
            or question.get("lecture_context")
            or ""
        ).strip()
        rag_context = question.get("rag_context") or question.get("ragContexts") or []
        
        type_names = {
            "single": "单选题",
            "multi": "多选题", 
            "judge": "判断题"
        }

        def format_answer() -> str:
            if q_type == "judge":
                if isinstance(answer, bool):
                    return "对" if answer else "错"
                return str(answer)

            if not options:
                return str(answer)

            def format_index(value) -> str:
                if isinstance(value, str) and len(value.strip()) == 1 and value.strip().isalpha():
                    index = ord(value.strip().upper()) - 65
                    if 0 <= index < len(options):
                        return f"{chr(65 + index)}. {options[index]}"
                    return value
                try:
                    index = int(value)
                except (TypeError, ValueError):
                    return str(value)
                if 0 <= index < len(options):
                    return f"{chr(65 + index)}. {options[index]}"
                return str(value)

            if isinstance(answer, list):
                return "、".join(format_index(item) for item in answer)

            if isinstance(answer, str) and re.fullmatch(r"[A-Za-z]+", answer.strip()) and len(answer.strip()) > 1:
                return "、".join(format_index(item) for item in answer.strip())

            return format_index(answer)
        
        prompt = f"""请为以下{type_names.get(q_type, '选择题')}生成解析：

题目：{content}
"""
        if options:
            for i, opt in enumerate(options):
                prompt += f"{chr(65+i)}. {opt}\n"

        if global_context:
            prompt += f"""
完整讲义参考：
{global_context[:120000]}
"""

        if isinstance(rag_context, list) and rag_context:
            context_lines = []
            for item in rag_context[:3]:
                if isinstance(item, dict):
                    text = str(item.get("text") or "").strip()
                    label = str(item.get("chapter") or item.get("id") or "讲义").strip()
                else:
                    text = str(item).strip()
                    label = "讲义"
                if text:
                    context_lines.append(f"- {label}: {text[:500]}")
            if context_lines:
                prompt += "\n讲义参考：\n" + "\n".join(context_lines) + "\n"
        
        prompt += f"""
正确答案：{format_answer()}

请严格围绕上面给出的正确答案生成一段简洁解析（50-100字），优先参考完整讲义参考，其次参考讲义参考中的命中片段；如果参考内容不相关，则只依据题目和正确答案解释。不要自行改判答案。直接给出解析内容，不要加"解析："前缀。"""
        
        return prompt
    
    @staticmethod
    async def generate_analysis_batch(
        provider: LLMProvider,
        questions: List[Dict],
        progress_callback: Optional[Callable[[int, int], Awaitable[None]]] = None
    ) -> List[Dict]:
        """
        批量生成解析 - 高并发版本
        
        使用信号量控制并发数，避免触发 API 速率限制
        """
        semaphore = asyncio.Semaphore(provider.config.max_concurrent)
        total = len(questions)
        completed = 0
        lock = asyncio.Lock()
        
        async def process_one(idx: int, question: Dict) -> Dict:
            """处理单个题目"""
            nonlocal completed
            
            if question.get("analysis"):
                # 已有解析，跳过
                async with lock:
                    completed += 1
                    if progress_callback:
                        await progress_callback(completed, total)
                return question
            
            async with semaphore:  # 限制并发
                try:
                    prompt = LLMService.build_prompt(question)
                    analysis = await provider.generate_with_retry(prompt)
                    question["analysis"] = analysis.strip()
                    
                    # 短暂延迟，避免触发速率限制
                    await asyncio.sleep(0.2)
                    
                except Exception as e:
                    print(f"题目 {idx+1} 生成解析失败: {e}")
                    question["analysis"] = f"生成解析失败: {str(e)[:50]}"
                
                async with lock:
                    completed += 1
                    if progress_callback:
                        await progress_callback(completed, total)
                
                return question
        
        # 并发执行所有任务
        tasks = [process_one(i, q) for i, q in enumerate(questions)]
        results = await asyncio.gather(*tasks)
        
        return results
    
    @staticmethod
    async def generate_analysis_with_multi_keys(
        provider_configs: List[LLMConfig],
        questions: List[Dict],
        progress_callback: Optional[Callable[[int, int], Awaitable[None]]] = None
    ) -> List[Dict]:
        """
        使用多组配置（多 Key）并发生成解析
        
        将题目分配到不同的 provider 上并行处理
        """
        if not provider_configs:
            raise ValueError("至少需要提供一个 LLMConfig")
        
        providers = [LLMService.create(cfg) for cfg in provider_configs]

        try:
            n_providers = len(providers)
            question_groups: List[List[Tuple[int, Dict]]] = [[] for _ in range(n_providers)]
            for i, q in enumerate(questions):
                question_groups[i % n_providers].append((i, q))

            all_results: List[Optional[Dict]] = [None] * len(questions)
            completed = 0
            lock = asyncio.Lock()

            async def wrapped_progress(_: int, __: int) -> None:
                nonlocal completed
                if not progress_callback:
                    return
                async with lock:
                    completed += 1
                    await progress_callback(completed, len(questions))

            async def run_group(group_provider: LLMProvider, group: List[Tuple[int, Dict]]) -> None:
                group_questions = [q for _, q in group]
                group_results = await LLMService.generate_analysis_batch(
                    group_provider,
                    group_questions,
                    progress_callback=wrapped_progress
                )
                for (origin_idx, _), result in zip(group, group_results):
                    all_results[origin_idx] = result

            tasks = [
                run_group(providers[i], group)
                for i, group in enumerate(question_groups)
                if group
            ]
            await asyncio.gather(*tasks)

            return [r if r is not None else questions[idx] for idx, r in enumerate(all_results)]
        finally:
            await LLMService.close_providers(providers)


# 测试
if __name__ == "__main__":
    async def test():
        # 测试单 Key
        config = LLMConfig(
            provider="siliconflow",
            api_keys=[os.getenv("SILICONFLOW_API_KEY", "test-key")],
            max_concurrent=3,
            max_retries=2
        )
        
        provider = LLMService.create(config)
        
        # 模拟 5 道题
        test_questions = [
            {
                "type": "single",
                "content": f"测试题目 {i+1}",
                "options": ["选项A", "选项B", "选项C", "选项D"],
                "answer": "B"
            }
            for i in range(5)
        ]
        
        async def progress(current, total):
            print(f"进度: {current}/{total}")
        
        print("测试批量生成...")
        # 由于可能是测试 key，这里只打印 prompt
        prompt = LLMService.build_prompt(test_questions[0])
        print(f"Prompt 示例:\n{prompt}")
    
    asyncio.run(test())
