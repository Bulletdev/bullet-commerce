# Documentação - Go Cart API

Bem-vindo à documentação técnica do projeto Go Cart API. Este diretório contém toda a documentação arquitetural e técnica do sistema.

## 📁 Arquivos de Documentação

### 🏗️ [architecture.md](./architecture.md)
Documentação completa da arquitetura do sistema, incluindo:

- **Diagrama de Arquitetura Geral**: Visão macro do sistema com todas as camadas
- **Fluxo de Autenticação**: Sequência detalhada dos processos de login/registro
- **Modelo de Dados (ERD)**: Estrutura do banco de dados com relacionamentos
- **Fluxo de Requests**: Como as requisições são processadas
- **Estrutura de Pastas**: Organização do código e responsabilidades
- **Tecnologias Utilizadas**: Stack tecnológico completo
- **Padrões Arquiteturais**: Design patterns implementados

## 🎯 Como Visualizar os Diagramas

Os diagramas estão criados em formato **Mermaid**, que pode ser visualizado em:

### GitHub
Os diagramas são renderizados automaticamente quando você visualiza os arquivos `.md` diretamente no GitHub.

### Editores Locais
- **VS Code**: Instale a extensão "Mermaid Preview"
- **IntelliJ/GoLand**: Suporte nativo para Mermaid
- **Online**: [Mermaid Live Editor](https://mermaid.live/)

### Documentação Online
- GitBook, Notion, ou qualquer plataforma que suporte Mermaid

## 🧩 Visão Geral da Arquitetura

Este projeto segue os princípios de **Clean Architecture** com as seguintes características:

- **Separação de Responsabilidades**: Cada camada tem função específica
- **Inversão de Dependências**: Interfaces bem definidas entre camadas  
- **Testabilidade**: Estrutura preparada para testes unitários e integração
- **Escalabilidade**: Arquitetura que facilita crescimento e manutenção

## 🚀 Stack Tecnológico Principal

- **Backend**: Go 1.23+ com Gorilla Mux
- **Database**: PostgreSQL via Supabase
- **Autenticação**: JWT + bcrypt
- **Deploy**: Render/Heroku
- **Testes**: Go testing + testify

## 📚 Para Desenvolvedores

Se você é novo no projeto, recomendamos a leitura na seguinte ordem:

1. **README principal** do projeto para setup inicial
2. **[architecture.md](./architecture.md)** para entender a estrutura
3. **Código fonte** começando por `cmd/main.go`
4. **Testes** em `internal/handlers/*_test.go`

## 🔄 Manutenção da Documentação

Esta documentação deve ser atualizada sempre que:
- Novos componentes forem adicionados
- A arquitetura for modificada
- Novas dependências forem incluídas
- Padrões de desenvolvimento mudarem

## 📞 Suporte

Para dúvidas sobre a arquitetura ou documentação:
- Abra uma issue no repositório
- Entre em contato com a equipe de desenvolvimento