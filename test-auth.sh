#!/bin/bash
# Script de prueba para verificar la autenticación del Oracle Free Tier Watcher

set -e

# Colores para output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔍 Oracle Free Tier Watcher - Test de Autenticación"
echo ""

# Configuración
PORT="${PORT:-8088}"
BASE_URL="http://localhost:${PORT}"

# Verificar si el servidor está corriendo
echo "📡 Verificando conexión al servidor..."
if ! curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/health" | grep -q "200"; then
    echo -e "${RED}❌ El servidor no está corriendo en ${BASE_URL}${NC}"
    echo "   Inicialo con: ./watcher"
    exit 1
fi
echo -e "${GREEN}✅ Servidor activo${NC}"
echo ""

# Test 1: Health check (sin autenticación)
echo "Test 1: Health Check (público)"
HEALTH=$(curl -s "${BASE_URL}/health")
if echo "$HEALTH" | grep -q "ok"; then
    echo -e "${GREEN}✅ /health responde correctamente (sin autenticación)${NC}"
else
    echo -e "${RED}❌ /health falló${NC}"
fi
echo ""

# Test 2: Acceso sin API Key
echo "Test 2: /usage sin API Key"
STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/usage")
if [ "$STATUS_CODE" = "401" ]; then
    echo -e "${GREEN}✅ Endpoint protegido correctamente (HTTP 401)${NC}"
elif [ "$STATUS_CODE" = "200" ]; then
    echo -e "${YELLOW}⚠️  API_KEY no configurada - endpoint público${NC}"
    echo "   Configura API_KEY en .env para proteger los endpoints"
else
    echo -e "${RED}❌ Respuesta inesperada: HTTP $STATUS_CODE${NC}"
fi
echo ""

# Test 3: Acceso con API Key (si está configurada)
if [ -n "$API_KEY" ]; then
    echo "Test 3: /usage con API Key válida"
    STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: $API_KEY" "${BASE_URL}/usage")
    if [ "$STATUS_CODE" = "200" ]; then
        echo -e "${GREEN}✅ Autenticación exitosa${NC}"
    else
        echo -e "${RED}❌ Autenticación falló (HTTP $STATUS_CODE)${NC}"
    fi
    echo ""

    echo "Test 4: /usage con API Key inválida"
    STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: invalid-key" "${BASE_URL}/usage")
    if [ "$STATUS_CODE" = "401" ]; then
        echo -e "${GREEN}✅ API Key inválida rechazada correctamente${NC}"
    else
        echo -e "${RED}❌ Respuesta inesperada: HTTP $STATUS_CODE${NC}"
    fi
    echo ""
else
    echo -e "${YELLOW}⚠️  Variable API_KEY no configurada${NC}"
    echo "   Exporta tu API_KEY para probar la autenticación:"
    echo "   export API_KEY='tu-clave-aqui'"
    echo ""
fi

# Test 5: Verificar formato de respuesta
echo "Test 5: Formato de respuesta JSON"
RESPONSE=$(curl -s -H "X-API-Key: ${API_KEY:-}" "${BASE_URL}/limits")
if echo "$RESPONSE" | jq . > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Respuesta JSON válida${NC}"
else
    echo -e "${RED}❌ Respuesta no es JSON válido${NC}"
fi
echo ""

# Resumen
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Endpoints disponibles:"
echo "   GET ${BASE_URL}/health  - Health check (público)"
echo "   GET ${BASE_URL}/limits  - Límites Free Tier"
echo "   GET ${BASE_URL}/usage   - Uso detallado"
echo "   GET ${BASE_URL}/status  - Estado rápido"
echo ""
if [ -n "$API_KEY" ]; then
    echo -e "${GREEN}🔒 Autenticación: HABILITADA${NC}"
    echo "   Usa: curl -H \"X-API-Key: \$API_KEY\" ${BASE_URL}/usage"
else
    echo -e "${YELLOW}⚠️  Autenticación: DESHABILITADA${NC}"
    echo "   Configura API_KEY en .env para mayor seguridad"
fi
echo ""
