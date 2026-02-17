#!/bin/bash

# Cargar variables desde .env si existe
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs)
fi

# Verificar configuración
if [ -z "$CHECKLY_API_KEY" ] || [ -z "$CHECKLY_ACCOUNT_ID" ]; then
  echo "❌ Error: CHECKLY_API_KEY o CHECKLY_ACCOUNT_ID no están definidos en .env"
  exit 1
fi


# Lista de IDs de los checks a actualizar
CHECK_IDS=(
  "ab7b3917-ee27-44e9-a993-575b0adf68a0" # Coolify Control Panel
  "c456ecb8-2004-4e2c-9444-ed8634b947f5" # Oracle Free Tier Monitor
)

echo "⏳ Cambiando frecuencia a 60 minutos (1 hora) para todos los checks..."

for ID in "${CHECK_IDS[@]}"; do
  echo "Updating $ID..."
  RESPONSE=$(curl -s -X PATCH "https://api.checklyhq.com/v1/checks/$ID" \
    -H "Authorization: Bearer $CHECKLY_API_KEY" \
    -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID" \
    -H "Content-Type: application/json" \
    -d '{"frequency": 60}')
  
  if echo "$RESPONSE" | grep -q '"id"'; then
    echo "✅ Actualizado!"
  else
    echo "❌ Error actualizando $ID:"
    echo "$RESPONSE"
  fi
  echo "-----------------------------------"
done
