##
## EPITECH PROJECT, 2025
## Summit
## File description:
## main
##

import requests
import json
import sys
import os
import time

BASE_URL = "http://localhost:1234/v1"
MODEL_NAME = "llama-3.2-1b-instruct"
API_TIMEOUT = 120


def check_server_available():
    try:
        response = requests.get(f"{BASE_URL}/models", timeout=5)
        return response.status_code == 200
    except requests.RequestException:
        return False


def analyze_and_fix_code(code):
    tools = [
        {
            "type": "function",
            "function": {
                "name": "perform_code_analysis",
                "description": "Analyzes code for errors and suggests fixes",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "code": {"type": "string", "description": "The code to analyze"},
                        "language": {
                            "type": "string",
                            "description": "Programming language of the code",
                            "enum": ["python", "go", "javascript", "java", "c++"]
                        }
                    },
                    "required": ["code", "language"]
                }
            }
        }
    ]

    request_data = {
        "model": MODEL_NAME,
        "messages": [
            {"role": "system", "content": "You are an expert code debugging assistant."},
            {"role": "user", "content": f"Analyze and fix this code:\n```\n{code}\n```"}
        ],
        "tools": tools,
        "response_format": {
            "type": "json_schema",
            "json_schema": {
                "schema": {
                    "type": "object",
                    "properties": {
                        "original_code": {"type": "string"},
                        "fixed_code": {"type": "string"},
                        "explanation": {"type": "string"},
                        "language": {"type": "string"},
                        "error_type": {"type": "string"}
                    },
                    "required": ["fixed_code", "explanation", "language", "error_type"]
                }
            }
        },
        "temperature": 0.3,
        "stream": False
    }

    response = requests.post(f"{BASE_URL}/chat/completions", json=request_data, timeout=API_TIMEOUT)
    if response.status_code != 200:
        raise Exception(f"API request failed: {response.status_code}\n{response.text}")

    data = response.json()
    return json.loads(data["choices"][0]["message"]["content"])


def validate_and_save(filename, fix):
    confirmation = input("\nApply these changes? [y/N]: ").strip().lower()
    if confirmation != 'y':
        print("Operation cancelled.")
        return

    backup_filename = f"{filename}.{time.strftime('%Y%m%d%H%M%S')}.bak"
    os.rename(filename, backup_filename)
    print(f"Backup saved to {backup_filename}")

    with open(filename, "w", encoding="utf-8") as f:
        f.write(fix["fixed_code"])

    print("Update successful!")


def main():
    if not check_server_available():
        print(f"LM Studio server not available. Please ensure it's running at {BASE_URL}")
        sys.exit(1)

    if len(sys.argv) < 2:
        print("Usage: codefixer.py <filename>")
        sys.exit(1)

    filename = sys.argv[1]
    try:
        with open(filename, "r", encoding="utf-8") as f:
            content = f.read()
    except Exception as e:
        print(f"Error reading file: {e}")
        sys.exit(1)

    try:
        fix = analyze_and_fix_code(content)
        print("\n=== Code Fix Report ===")
        print(f"Language: {fix['language']}")
        print(f"Error Type: {fix['error_type']}")
        print("\nOriginal Code:")
        print(fix["original_code"])
        print("\nFixed Code:")
        print(fix["fixed_code"])
        print("\nExplanation:")
        print(fix["explanation"])

        validate_and_save(filename, fix)
    except Exception as e:
        print(f"Error fixing code: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
