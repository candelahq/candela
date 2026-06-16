#!/bin/bash
# Test all models from the LM Studio endpoint

curl -s http://127.0.0.1:1234/v1/models | jq -r '.data[].id' > /tmp/candela_models.txt
total=$(wc -l < /tmp/candela_models.txt | tr -d ' ')
echo "Testing $total models..."
echo ""

pass=0
fail=0

while IFS= read -r model; do
  printf "%-45s " "$model"
  result=$(curl -s --max-time 30 http://127.0.0.1:1234/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"'"$model"'","messages":[{"role":"user","content":"reply with exactly one word: ok"}],"max_tokens":5}' 2>&1)
  
  has_choices=$(echo "$result" | jq -e '.choices[0]' 2>/dev/null)
  if [ $? -eq 0 ]; then
    content=$(echo "$result" | jq -r '.choices[0].message.content // "empty"' | tr -d '\n' | head -c 30)
    echo "✅ $content"
    pass=$((pass + 1))
  else
    errMsg=$(echo "$result" | jq -r '
      if .error.message then .error.message
      elif type == "array" and .[0].error.message then .[0].error.message  
      elif .error then (.error | tostring)
      else "unknown error"
      end' 2>/dev/null | head -c 100)
    echo "❌ $errMsg"
    fail=$((fail + 1))
  fi
done < /tmp/candela_models.txt

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Results: $pass ✅  $fail ❌  (total: $total)"
