# Provider Configuration Guide

This guide provides step-by-step instructions for configuring a new data provider into the Orchestration Engine (OE)
which will enable OE to call the respective providers. The OE is designed to fetch data from multiple providers based
on consumer requests, and proper integration is essential for seamless operation. This guide explains how to set up a 
new provider, Registrar General Department of Farajaland (RGDF), as an example.

## Prerequisites

- As the first step, ensure you have obtained the access details (GraphQL endpoint URL, authentication credentials,
  etc.) from the data provider.
- Familiarize yourself with the provider's GraphQL schema and data structure.

## Step 1: Update Configuration

1. Open the `config.json` file located in the Orchestration Engine's root directory.
2. Add a new entry to the `providers` array with the following details:
    - `providerKey`: A unique identifier for the data provider (e.g., "rgdf").
    - `providerUrl`: The GraphQL endpoint URL of the data provider.
      Example:
   ```json
   {
     "providerKey": "rgdf",
     "providerUrl": "https://rgdf.gov.fl/graphql"
   }
   ```
3. Provider Auth (Optional): If the provider requires authentication, add the necessary credentials (API key,
   OAuth tokens, etc.) to the configuration file. Explained in the next step.

## Step 2: Auth Method Configuration

1. In the `config.json` file, locate the `providers` array.
2. For each provider that requires authentication, add an `auth` object with the following fields:
    - `type`: The type of authentication (e.g., "apiKey", "oauth2").

    1. For `apiKey` type:
        - `apiKeyName`: The name of the header where the API key should be included (e.g., "Authorization").
        - `apiKeyValue`: The actual API key provided by the data provider.
    2. For `oauth2` type:
        - `tokenUrl`: The URL to obtain the OAuth2 token.
        - `clientId`: The client ID provided by the data provider.
        - `clientSecret`: The client secret provided by the data provider.

## Step 3: Argument Mappings

1. In the `config.json` file, locate the `argMappings` array.
2. Add new entries to map arguments from the consumer's request to the provider's expected arguments. This is explained
   with an example below.

   Let's say the source query (consumer facing) query looks like this.
    ```graphql
    query getPersonInfo {
        personInfo(nic: "199512345678") {
            name
            address
            profession
            birthInfo {
                 brNo
             }
         }
   }
     ```  
   And the provider (rgdf) expects the query to look like this.
    ```graphql
    query GetPersonInforgdf {
        getPersonInfo(nic: "12") {
            fullName
            permanentAddress
            birthRegistrationNumber
        }
    }
   ```
   Example:
    The above scenario maps to the following arguments.
   ```json
   {
     "providerKey": "rgdf",
     "targetArgName": "nic",
     "sourceArgPath": "personInfo-nic",
     "targetArgPath": "rgdf.person"
   }
   ```
2. Repeat this process for all arguments that need to be mapped.

## Schema Directives

1. Ensure that the provider's GraphQL schema includes the `@sourceInfo` directive on each leaf field that the OE will
   query.
2. The `@sourceInfo` directive should contain the following fields:
    - `providerKey`: The unique identifier for the data provider (must match the `providerKey` in the configuration).
    - `providerField`: The field name in the provider's schema that corresponds to this field.
      Example:
   ```graphql
   type PersonInfo {
        name: String @sourceInfo(providerKey: "rgdf", providerField: "getPersonInfo.fullName")
        birthInfo: BirthInfo
   }
   
    type BirthInfo {
          brNo: String @sourceInfo(providerKey: "rgdf", providerField: "getPersonInfo.birthRegistrationNumber")
    }
   ```
   This will be mapped to the provider's schema as follows:
   ```graphql
   query PersonInfoQueryrgdf {
        getPersonInfo {
            fullName
            birthRegistrationNumber
        }
   }
   ```
3. Explore `schema.graphql` for further examples.