package server

import (
	"strings"
	"github.com/graphql-go/graphql"
	"github.com/daptin/daptin/server/resource"
	"log"
	"github.com/graphql-go/relay"
	"golang.org/x/net/context"
	"github.com/artpar/api2go"
	"net/http"
	"encoding/base64"
	"encoding/json"
	"errors"
	//"fmt"
)

// Capitalize capitalizes the first character of the string.
func Capitalize(s string) string {
	if len(s) == 1 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

//var todoType *graphql.Object
//var userType *graphql.Object

var nodeDefinitions *relay.NodeDefinitions
var todosConnection *relay.GraphQLConnectionDefinitions

var Schema graphql.Schema

func MakeGraphqlSchema(cmsConfig *resource.CmsConfig, resources map[string]*resource.DbResource) *graphql.Schema {

	//mutations := make(graphql.InputObjectConfigFieldMap)
	//query := make(graphql.InputObjectConfigFieldMap)
	//done := make(map[string]bool)

	inputTypesMap := make(map[string]*graphql.Object)
	//outputTypesMap := make(map[string]graphql.Output)
	//connectionMap := make(map[string]*relay.GraphQLConnectionDefinitions)
	nodeDefinitions = relay.NewNodeDefinitions(relay.NodeDefinitionsConfig{
		IDFetcher: func(id string, info graphql.ResolveInfo, ctx context.Context) (interface{}, error) {
			resolvedID := relay.FromGlobalID(id)
			pr := &http.Request{
				Method: "GET",
			}
			pr = pr.WithContext(ctx)
			req := api2go.Request{
				PlainRequest: pr,
			}
			responder, err := resources[strings.ToLower(resolvedID.Type)].FindOne(resolvedID.ID, req)
			if responder.Result() != nil {
				return responder.Result().(api2go.Api2GoModel).Data, err
			}
			return nil, err

		},
		TypeResolve: func(p graphql.ResolveTypeParams) *graphql.Object {
			log.Printf("Type resolve query: %v", p)
			//return inputTypesMap[p.Value]
			return nil
		},
	})
	rootFields := make(graphql.Fields)

	for _, table := range cmsConfig.Tables {


	allFields := make(graphql.FieldConfigArgument)
	uniqueFields := make(graphql.FieldConfigArgument)

		if strings.Contains(table.TableName, "_has_") {
			continue
		}
		fields := make(graphql.Fields)

		for _, column := range table.Columns {

			//if column.IsForeignKey {
			//	continue
			//}

			allFields[table.TableName+"."+column.ColumnName] = &graphql.ArgumentConfig{
				Type: resource.ColumnManager.GetGraphqlType(column.ColumnType),
			}

			if column.IsUnique || column.IsPrimaryKey {
				uniqueFields[table.TableName+"."+column.ColumnName] = &graphql.ArgumentConfig{
					Type: resource.ColumnManager.GetGraphqlType(column.ColumnType),
				}
			}

			graphqlType := resource.ColumnManager.GetGraphqlType(column.ColumnType)
			log.Printf("Add column: %v == %v", table.TableName+"."+column.ColumnName, graphqlType)
			//done[table.TableName+"."+column.ColumnName] = true
			fields[column.ColumnName] = &graphql.Field{
				Type:        graphqlType,
				Description: column.ColumnDescription,
			}
		}
		fields["id"] = &graphql.Field{
			Description: "The ID of an object",
			Type:        graphql.NewNonNull(graphql.ID),
		}

		todoType := graphql.NewObject(graphql.ObjectConfig{
			Name:   table.TableName,
			Fields: fields,
		})



		inputTypesMap[table.TableName] = todoType

		//
		//rootFields[table.TableName] = &graphql.Field{
		//	Type:        inputTypesMap[table.TableName],
		//	Args:        graphql.FieldConfigArgument{},
		//	Description: fmt.Sprintf("Get a single %v", table.TableName),
		//	Resolve: func(p graphql.ResolveParams) (interface{}, error) {
		//		log.Printf("Resolve %v", p.Args)
		//		//req := api2go.
		//		//resources[table.TableName].PaginatedFindAll(req)
		//		return nil, nil
		//	},
		//}
		//


		rootFields[table.TableName] = &graphql.Field{
			Type:        graphql.NewList(inputTypesMap[table.TableName]),
			Description: "Find all " + table.TableName,
			Args:        graphql.FieldConfigArgument{},
			//Args:        uniqueFields,
			Resolve: func(table resource.TableInfo) (func(params graphql.ResolveParams) (interface{}, error)) {
				return func(params graphql.ResolveParams) (interface{}, error) {

					log.Printf("Arguments: %v", params.Args)

					filters := make([]resource.Query, 0)

					for keyName, value := range params.Args {

						if _, ok := uniqueFields[keyName]; !ok {
							continue
						}

						query := resource.Query{
							ColumnName: keyName,
							Operator:   "is",
							Value:      value.(string),
						}
						filters = append(filters, query)
					}

					pr := &http.Request{
						Method: "GET",
					}
					pr = pr.WithContext(params.Context)


					jsStr, err := json.Marshal(filters)
					req := api2go.Request{
						PlainRequest: pr,

						QueryParams: map[string][]string{
							"query": {base64.StdEncoding.EncodeToString(jsStr)},
						},
					}

					count, responder, err := resources[table.TableName].PaginatedFindAll(req)

					if count == 0 {
						return nil, errors.New("no such entity")
					}
					items := make([]map[string]interface{}, 0)

					results := responder.Result().([]*api2go.Api2GoModel)

					for _, r := range results {
						items = append(items, r.Data)
					}

					return items, err

				}
			}(table),
		}
		//
		//rootFields["all"+Capitalize(inflector.Pluralize(table.TableName))] = &graphql.Field{
		//	Type:        graphql.NewList(inputTypesMap[table.TableName]),
		//	Description: "Get a list of " + inflector.Pluralize(table.TableName),
		//	Args:        allFields,
		//	Resolve: func(table resource.TableInfo) (func(params graphql.ResolveParams) (interface{}, error)) {
		//
		//		return func(params graphql.ResolveParams) (interface{}, error) {
		//			log.Printf("Arguments: %v", params.Args)
		//
		//			filters := make([]resource.Query, 0)
		//
		//			for keyName, value := range params.Args {
		//
		//				if _, ok := uniqueFields[keyName]; !ok {
		//					continue
		//				}
		//
		//				query := resource.Query{
		//					ColumnName: keyName,
		//					Operator:   "is",
		//					Value:      value.(string),
		//				}
		//				filters = append(filters, query)
		//			}
		//
		//			pr := &http.Request{
		//				Method: "GET",
		//			}
		//			pr = pr.WithContext(params.Context)
		//			jsStr, err := json.Marshal(filters)
		//			req := api2go.Request{
		//				PlainRequest: pr,
		//				QueryParams: map[string][]string{
		//					"query":              {base64.StdEncoding.EncodeToString(jsStr)},
		//					"included_relations": {"*"},
		//				},
		//			}
		//
		//			count, responder, err := resources[table.TableName].PaginatedFindAll(req)
		//
		//			if count == 0 {
		//				return nil, errors.New("no such entity")
		//			}
		//
		//			items := responder.Result().([]*api2go.Api2GoModel)
		//
		//			results := make([]map[string]interface{}, 0)
		//			for _, item := range items {
		//				ai := item
		//
		//				dataMap := ai.Data
		//
		//				includedMap := make(map[string]interface{})
		//
		//				for _, includedObject := range ai.Includes {
		//					id := includedObject.GetID()
		//					includedMap[id] = includedObject.GetAttributes()
		//				}
		//
		//				for _, relation := range table.Relations {
		//					columnName := relation.GetSubjectName()
		//					if table.TableName == relation.Subject {
		//						columnName = relation.GetObjectName()
		//					}
		//					referencedObjectId := dataMap[columnName]
		//					if referencedObjectId == nil {
		//						continue
		//					}
		//					dataMap[columnName] = includedMap[referencedObjectId.(string)]
		//				}
		//
		//				results = append(results, dataMap)
		//			}
		//			return results, err
		//		}
		//	}(table),
		//}
		//
		//rootFields["meta"+Capitalize(inflector.Pluralize(table.TableName))] = &graphql.Field{
		//	Type:        graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
		//		//Name
		//	})),
		//	Description: "Aggregates for " + inflector.Pluralize(table.TableName),
		//	Args: graphql.FieldConfigArgument{
		//		"group": &graphql.ArgumentConfig{
		//			Type: graphql.NewList(graphql.String),
		//		},
		//		"join": &graphql.ArgumentConfig{
		//			Type: graphql.NewList(graphql.String),
		//		},
		//		"column": &graphql.ArgumentConfig{
		//			Type: graphql.NewList(graphql.String),
		//		},
		//		"order": &graphql.ArgumentConfig{
		//			Type: graphql.NewList(graphql.String),
		//		},
		//	},
		//	Resolve: func(table resource.TableInfo) (func(params graphql.ResolveParams) (interface{}, error)) {
		//
		//		return func(params graphql.ResolveParams) (interface{}, error) {
		//			log.Printf("Arguments: %v", params.Args)
		//			aggReq := resource.AggregationRequest{}
		//
		//			aggReq.RootEntity = table.TableName
		//
		//			if params.Args["group"] != nil {
		//				groupBys := params.Args["group"].([]interface{})
		//				aggReq.GroupBy = make([]string, 0)
		//				for _, grp := range groupBys {
		//					aggReq.GroupBy = append(aggReq.GroupBy, grp.(string))
		//				}
		//			}
		//			if params.Args["join"] != nil {
		//				groupBys := params.Args["join"].([]interface{})
		//				aggReq.Join = make([]string, 0)
		//				for _, grp := range groupBys {
		//					aggReq.Join = append(aggReq.Join, grp.(string))
		//				}
		//			}
		//			if params.Args["column"] != nil {
		//				groupBys := params.Args["column"].([]interface{})
		//				aggReq.ProjectColumn = make([]string, 0)
		//				for _, grp := range groupBys {
		//					aggReq.ProjectColumn = append(aggReq.ProjectColumn, grp.(string))
		//				}
		//			}
		//			if params.Args["order"] != nil {
		//				groupBys := params.Args["order"].([]interface{})
		//				aggReq.Order = make([]string, 0)
		//				for _, grp := range groupBys {
		//					aggReq.Order = append(aggReq.Order, grp.(string))
		//				}
		//			}
		//
		//			//params.Args["query"].(string)
		//			//aggReq.Query =
		//
		//			aggResponse, err := resources[table.TableName].DataStats(aggReq)
		//			return aggResponse, err
		//		}
		//	}(table),
		//}
	}
	//rootFields["node"] = nodeDefinitions.NodeField

	//rootQuery := graphql.NewObject(graphql.ObjectConfig{
	//	Name:   "RootQuery",
	//	Fields: rootFields,
	//})



	// root query
	// we just define a trivial example here, since root query is required.
	// Test with curl
	// curl -g 'http://localhost:8080/graphql?query={lastTodo{id,text,done}}'
	var rootQuery = graphql.NewObject(graphql.ObjectConfig{
		Name: "RootQuery",
		Fields: rootFields,
	})


	//addTodoMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "AddTodo",
	//	InputFields: graphql.InputObjectConfigFieldMap{
	//		"text": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.String),
	//		},
	//	},
	//	OutputFields: graphql.Fields{
	//		"todoEdge": &graphql.Field{
	//			Type: todosConnection.EdgeType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				//payload, _ := p.Source.(map[string]interface{})
	//				//todoId, _ := payload["todoId"].(string)
	//				//todo := GetTodo(todoId)
	//				return relay.EdgeType{
	//					Node: nil,
	//					//Cursor: relay.CursorForObjectInConnection(TodosToSliceInterface(GetTodos("any")), todo),
	//				}, nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		//text, _ := inputMap["text"].(string)
	//		//todoId := nil
	//		return map[string]interface{}{
	//			"todoId": "todo-refid",
	//		}, nil
	//	},
	//})

	//changeTodoStatusMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "ChangeTodoStatus",
	//	InputFields: graphql.InputObjectConfigFieldMap{
	//		"id": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.ID),
	//		},
	//		"complete": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.Boolean),
	//		},
	//	},
	//	OutputFields: graphql.Fields{
	//		"todo": &graphql.Field{
	//			Type: todoType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				//payload, _ := p.Source.(map[string]interface{})
	//				//todoId, _ := payload["todoId"].(string)
	//				//todo := nil
	//				return nil, nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		//id, _ := inputMap["id"].(string)
	//		//complete, _ := inputMap["complete"].(bool)
	//		//resolvedId := relay.FromGlobalID(id)
	//		//ChangeTodoStatus(resolvedId.ID, complete)
	//		return map[string]interface{}{
	//			"todoId": "todo-ref-id",
	//		}, nil
	//	},
	//})

	//markAllTodosMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "MarkAllTodos",
	//	InputFields: graphql.InputObjectConfigFieldMap{
	//		"complete": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.Boolean),
	//		},
	//	},
	//	OutputFields: graphql.Fields{
	//		"changedTodos": &graphql.Field{
	//			Type: graphql.NewList(todoType),
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				//payload, _ := p.Source.(map[string]interface{})
	//				//todoIds, _ := payload["todoIds"].([]string)
	//				//todos := []*interface{}{}
	//				//for _, todoId := range todoIds {
	//				//	todo := nil
	//				//	if todo != nil {
	//				//		todos = append(todos, todo)
	//				//	}
	//				//}
	//				return nil, nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		//complete, _ := inputMap["complete"].(bool)
	//		//todoIds := nil
	//		return map[string]interface{}{
	//			"todoIds": "todi-ids",
	//		}, nil
	//	},
	//})

	//removeCompletedTodosMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "RemoveCompletedTodos",
	//	OutputFields: graphql.Fields{
	//		"deletedTodoIds": &graphql.Field{
	//			Type: graphql.NewList(graphql.String),
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				payload, _ := p.Source.(map[string]interface{})
	//				return payload["todoIds"], nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		todoIds := []string{}
	//		return map[string]interface{}{
	//			"todoIds": todoIds,
	//		}, nil
	//	},
	//})

	//removeTodoMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "RemoveTodo",
	//	InputFields: graphql.InputObjectConfigFieldMap{
	//		"id": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.ID),
	//		},
	//	},
	//	OutputFields: graphql.Fields{
	//		"deletedTodoId": &graphql.Field{
	//			Type: graphql.ID,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				payload, _ := p.Source.(map[string]interface{})
	//				return payload["todoId"], nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		id, _ := inputMap["id"].(string)
	//		resolvedId := relay.FromGlobalID(id)
	//		//RemoveTodo(resolvedId.ID)
	//		return map[string]interface{}{
	//			"todoId": resolvedId.ID,
	//		}, nil
	//	},
	//})
	//renameTodoMutation := relay.MutationWithClientMutationID(relay.MutationConfig{
	//	Name: "RenameTodo",
	//	InputFields: graphql.InputObjectConfigFieldMap{
	//		"id": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.ID),
	//		},
	//		"text": &graphql.InputObjectFieldConfig{
	//			Type: graphql.NewNonNull(graphql.String),
	//		},
	//	},
	//	OutputFields: graphql.Fields{
	//		"todo": &graphql.Field{
	//			Type: todoType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				//payload, _ := p.Source.(map[string]interface{})
	//				//todoId, _ := payload["todoId"].(string)
	//				return nil, nil
	//			},
	//		},
	//		"viewer": &graphql.Field{
	//			Type: userType,
	//			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
	//				return nil, nil
	//			},
	//		},
	//	},
	//	MutateAndGetPayload: func(inputMap map[string]interface{}, info graphql.ResolveInfo, ctx context.Context) (map[string]interface{}, error) {
	//		id, _ := inputMap["id"].(string)
	//		resolvedId := relay.FromGlobalID(id)
	//		//text, _ := inputMap["text"].(string)
	//		//RenameTodo(resolvedId.ID, text)
	//		return map[string]interface{}{
	//			"todoId": resolvedId.ID,
	//		}, nil
	//	},
	//})
	//mutationType := graphql.NewObject(graphql.ObjectConfig{
	//	Name: "Mutation",
	//	Fields: graphql.Fields{
	//		"addTodo":              addTodoMutation,
	//		"changeTodoStatus":     changeTodoStatusMutation,
	//		"markAllTodos":         markAllTodosMutation,
	//		"removeCompletedTodos": removeCompletedTodosMutation,
	//		"removeTodo":           removeTodoMutation,
	//		"renameTodo":           renameTodoMutation,
	//	},
	//})

	var err error
	Schema, err = graphql.NewSchema(graphql.SchemaConfig{
		Query: rootQuery,
		//Mutation: mutationType,
	})
	if err != nil {
		panic(err)
	}

	return &Schema

	//for _, table := range cmsConfig.Tables {
	//
	//	for _, relation := range table.Relations {
	//		if relation.Relation == "has_one" || relation.Relation == "belongs_to" {
	//			if relation.Subject == table.TableName {
	//				log.Printf("Add column: %v", table.TableName+"."+relation.GetObjectName())
	//				if done[table.TableName+"."+relation.GetObjectName()] {
	//					continue
	//					panic("ok")
	//				}
	//				done[table.TableName+"."+relation.GetObjectName()] = true
	//				inputTypesMap[table.TableName].Fields()[table.TableName+"."+relation.GetObjectName()] = &graphql.InputObjectField{
	//					Type:        inputTypesMap[relation.GetObject()],
	//					PrivateName: relation.GetObjectName(),
	//				}
	//			} else {
	//				log.Printf("Add column: %v", table.TableName+"."+relation.GetSubjectName())
	//				if done[table.TableName+"."+relation.GetSubjectName()] {
	//					// panic("ok")
	//				}
	//				done[table.TableName+"."+relation.GetSubjectName()] = true
	//				inputTypesMap[table.TableName].Fields()[table.TableName+"."+relation.GetSubjectName()] = &graphql.InputObjectField{
	//					Type:        inputTypesMap[relation.GetSubject()],
	//					PrivateName: relation.GetSubjectName(),
	//				}
	//			}
	//
	//		} else {
	//			if relation.Subject == table.TableName {
	//				log.Printf("Add column: %v", table.TableName+"."+relation.GetObjectName())
	//				if done[table.TableName+"."+relation.GetObjectName()] {
	//					panic("ok")
	//				}
	//				done[table.TableName+"."+relation.GetObjectName()] = true
	//				inputTypesMap[table.TableName].Fields()[table.TableName+"."+relation.GetObjectName()] = &graphql.InputObjectField{
	//					PrivateName: relation.GetObjectName(),
	//					Type:        graphql.NewList(inputTypesMap[relation.GetObject()]),
	//				}
	//			} else {
	//				log.Printf("Add column: %v", table.TableName+"."+relation.GetSubjectName())
	//				if done[table.TableName+"."+relation.GetSubjectName()] {
	//					panic("ok")
	//				}
	//				done[table.TableName+"."+relation.GetSubjectName()] = true
	//				inputTypesMap[table.TableName].Fields()[table.TableName+"."+relation.GetSubjectName()] = &graphql.InputObjectField{
	//					Type:        graphql.NewList(inputTypesMap[relation.GetSubject()]),
	//					PrivateName: relation.GetSubjectName(),
	//				}
	//			}
	//		}
	//	}
	//}

	//for _, table := range cmsConfig.Tables {
	//
	//	createFields := make(graphql.FieldConfigArgument)
	//
	//	for _, column := range table.Columns {
	//
	//		if column.IsForeignKey {
	//			continue
	//		}
	//
	//

	//
	//		if IsStandardColumn(column.ColumnName) {
	//			continue
	//		}
	//
	//		if column.IsForeignKey {
	//			continue
	//		}
	//
	//		if column.IsNullable {
	//			createFields[table.TableName+"."+column.ColumnName] = &graphql.ArgumentConfig{
	//				Type: resource.ColumnManager.GetGraphqlType(column.ColumnType),
	//			}
	//		} else {
	//			createFields[table.TableName+"."+column.ColumnName] = &graphql.ArgumentConfig{
	//				Type: graphql.NewNonNull(resource.ColumnManager.GetGraphqlType(column.ColumnType)),
	//			}
	//		}
	//
	//	}
	//
	//	//for _, relation := range table.Relations {
	//	//
	//	//	if relation.Relation == "has_one" || relation.Relation == "belongs_to" {
	//	//		if relation.Subject == table.TableName {
	//	//			allFields[table.TableName+"."+relation.GetObjectName()] = &graphql.ArgumentConfig{
	//	//				Type: inputTypesMap[relation.GetObject()],
	//	//			}
	//	//		} else {
	//	//			allFields[table.TableName+"."+relation.GetSubjectName()] = &graphql.ArgumentConfig{
	//	//				Type: inputTypesMap[relation.GetSubject()],
	//	//			}
	//	//		}
	//	//
	//	//	} else {
	//	//		if relation.Subject == table.TableName {
	//	//			allFields[table.TableName+"."+relation.GetObjectName()] = &graphql.ArgumentConfig{
	//	//				Type: graphql.NewList(inputTypesMap[relation.GetObject()]),
	//	//			}
	//	//		} else {
	//	//			allFields[table.TableName+"."+relation.GetSubjectName()] = &graphql.ArgumentConfig{
	//	//				Type: graphql.NewList(inputTypesMap[relation.GetSubject()]),
	//	//			}
	//	//		}
	//	//	}
	//	//}
	//
	//	//mutations["create"+Capitalize(table.TableName)] = &graphql.InputObjectFieldConfig{
	//	//	Type:        inputTypesMap[table.TableName],
	//	//	Description: "Create a new " + table.TableName,
	//	//	//Args:        createFields,
	//	//	//Resolve: func(table resource.TableInfo) (func(params graphql.ResolveParams) (interface{}, error)) {
	//	//	//	return func(p graphql.ResolveParams) (interface{}, error) {
	//	//	//		log.Printf("create resolve params: %v", p)
	//	//	//
	//	//	//		data := make(map[string]interface{})
	//	//	//
	//	//	//		for key, val := range p.Args {
	//	//	//			data[key] = val
	//	//	//		}
	//	//	//
	//	//	//		model := api2go.NewApi2GoModelWithData(table.TableName, nil, 0, nil, data)
	//	//	//
	//	//	//		pr := &http.Request{
	//	//	//			Method: "PATCH",
	//	//	//		}
	//	//	//		pr = pr.WithContext(p.Context)
	//	//	//		req := api2go.Request{
	//	//	//			PlainRequest: pr,
	//	//	//			QueryParams: map[string][]string{
	//	//	//			},
	//	//	//		}
	//	//	//
	//	//	//		res, err := resources[table.TableName].Create(model, req)
	//	//	//
	//	//	//		return res.Result().(api2go.Api2GoModel).Data, err
	//	//	//	}
	//	//	//}(table),
	//	//}
	//
	//	//mutations["update"+Capitalize(table.TableName)] = &graphql.InputObjectFieldConfig{
	//	//	Type:        inputTypesMap[table.TableName],
	//	//	Description: "Create a new " + table.TableName,
	//	//	//Args:        createFields,
	//	//	//Resolve: func(table resource.TableInfo) (func(params graphql.ResolveParams) (interface{}, error)) {
	//	//	//	return func(p graphql.ResolveParams) (interface{}, error) {
	//	//	//		log.Printf("create resolve params: %v", p)
	//	//	//
	//	//	//		data := make(map[string]interface{})
	//	//	//
	//	//	//		for key, val := range p.Args {
	//	//	//			data[key] = val
	//	//	//		}
	//	//	//
	//	//	//		model := api2go.NewApi2GoModelWithData(table.TableName, nil, 0, nil, data)
	//	//	//
	//	//	//		pr := &http.Request{
	//	//	//			Method: "PATCH",
	//	//	//		}
	//	//	//		pr = pr.WithContext(p.Context)
	//	//	//		req := api2go.Request{
	//	//	//			PlainRequest: pr,
	//	//	//			QueryParams: map[string][]string{
	//	//	//			},
	//	//	//		}
	//	//	//
	//	//	//		res, err := resources[table.TableName].Update(model, req)
	//	//	//
	//	//	//		return res.Result().(api2go.Api2GoModel).Data, err
	//	//	//	}
	//	//	//}(table),
	//	//}
	//



	//
	//var rootMutation = graphql.NewObject(graphql.ObjectConfig{
	//	Name:   "RootMutation",
	//	Fields: mutations,
	//});
	//var rootQuery = graphql.NewObject(graphql.ObjectConfig{
	//	Name:   "RootQuery",
	//	Fields: query,
	//})
	//
	//// define schema, with our rootQuery and rootMutation
	//var schema, _ = graphql.NewSchema(graphql.SchemaConfig{
	//	Query:    rootQuery,
	//	Mutation: rootMutation,
	//})
	//
	//return &schema

}
